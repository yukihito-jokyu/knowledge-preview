import { execFile } from "node:child_process";
import { mkdtemp, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { promisify } from "node:util";
import { test as base, type Page, type Video } from "@playwright/test";

const execute = promisify(execFile);

// Firefoxの信頼ストアはテストごとの専用profileへ作る。認証状態は共有しない。
export const test = base.extend({
  context: async (
    { browserName, context, playwright, contextOptions, baseURL, viewport, video },
    provideContext,
    testInfo,
  ) => {
    const certificate = process.env.E2E_CA_CERT;
    if (browserName !== "firefox" || !certificate) {
      await provideContext(context);
      return;
    }
    const mode = typeof video === "string" ? video : video.mode;
    // 独自contextはランナーの自動録画を経由しないため、録画と添付をここで管理する。
    if (mode !== "on" && mode !== "off") {
      throw new Error(
        "Firefoxの共通fixtureはvideoのon/offに対応しています。別モードを使う場合は保存条件も実装してください。",
      );
    }
    const videos: Video[] = [];
    const profile = await mkdtemp(join(tmpdir(), "knowledge-e2e-firefox-"));
    try {
      await execute("certutil", ["-N", "--empty-password", "-d", `sql:${profile}`]);
      await execute("certutil", [
        "-A",
        "-d",
        `sql:${profile}`,
        "-n",
        "knowledge-e2e",
        "-t",
        "C,,",
        "-i",
        certificate,
      ]);

      const trustedContext = await playwright.firefox.launchPersistentContext(profile, {
        ...contextOptions,
        baseURL,
        viewport,
        ignoreHTTPSErrors: false,
        recordVideo:
          mode === "on"
            ? {
                dir: testInfo.outputPath("videos"),
                size: typeof video === "object" ? video.size : undefined,
              }
            : undefined,
      });

      const collectVideo = (page: Page) => {
        const recording = page.video();
        if (recording) videos.push(recording);
      };

      // テストが新しく開いたページの動画だけを添付する。
      trustedContext.on("page", collectVideo);
      try {
        await provideContext(trustedContext);
      } finally {
        await trustedContext.close();
        for (const recording of videos) {
          await testInfo.attach("video", {
            path: await recording.path(),
            contentType: "video/webm",
          });
        }
      }
    } finally {
      await rm(profile, { recursive: true, force: true });
    }
  },
});

export { expect } from "@playwright/test";
