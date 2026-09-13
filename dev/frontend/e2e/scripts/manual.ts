import { access, mkdtemp, readFile, rm } from "node:fs/promises";
import { createHash, X509Certificate } from "node:crypto";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { fileURLToPath } from "node:url";
import { DatabaseSync } from "node:sqlite";
import { expect, chromium } from "@playwright/test";
import { authenticateTestSession, assertCookieIsHostOnly } from "../fixtures/auth/test-session.ts";

const origin = "https://app.knowledge.test";
const certificate = fileURLToPath(new URL("../.manual/ca.crt", import.meta.url));
const check = process.argv.includes("--check");
await access(certificate).catch(() => {
  throw new Error("先に task manual:up を実行してください。");
});
const profile = await mkdtemp(join(tmpdir(), "knowledge-manual-"));

const args = [
  "--host-resolver-rules=MAP app.knowledge.test 127.0.0.1, MAP preview.knowledge.test 127.0.0.1, MAP oauth.knowledge.test 127.0.0.1",
  "--no-proxy-server",
];

try {
  // Chromium自身に専用profileの証明書ストアを初期化させる。
  const setup = await chromium.launchPersistentContext(profile, {
    channel: "chromium",
    headless: true,
    args,
  });

  try {
    const page = setup.pages()[0];
    await page.goto("chrome://certificate-manager/localcerts/usercerts");
    await expect(page.locator("certificate-list:visible")).toHaveCount(3);
    if (check) await expect(page.goto(origin)).rejects.toThrow("ERR_CERT_AUTHORITY_INVALID");
  } finally {
    await setup.close();
  }

  const ca = new X509Certificate(await readFile(certificate));
  if (!ca.ca) throw new Error("検証環境のCA証明書ではありません。");
  const database = new DatabaseSync(join(profile, "Default", "ServerCertificate"));
  try {
    // ponytail: Chromiumのストアv1に限定。Playwright更新時はmanual:checkで形式を再確認する。
    if (database.prepare("SELECT value FROM meta WHERE key = 'version'").get()?.value !== "1") {
      throw new Error("Chromiumの証明書ストア形式が変更されています。");
    }
    // CertificateMetadata { trust { trust_type: TRUSTED(3) } } のprotobuf。
    // https://chromium.googlesource.com/chromium/src/+/refs/tags/143.0.7491.0/components/server_certificate_database/server_certificate_database.proto
    database
      .prepare("INSERT INTO certificates(sha256hash_hex, der_cert, trust_settings) VALUES(?, ?, ?)")
      .run(
        createHash("sha256").update(ca.raw).digest("hex"),
        ca.raw,
        Buffer.from([0x0a, 0x02, 0x08, 0x03]),
      );
  } finally {
    database.close();
  }

  // CAと名前解決はこの使い捨てprofileだけに適用する。TLS検証は有効のまま。
  const context = await chromium.launchPersistentContext(profile, {
    channel: "chromium",
    headless: check,
    handleSIGINT: false,
    handleSIGTERM: false,
    viewport: { width: 1440, height: 1000 },
    ignoreHTTPSErrors: false,
    args,
  });

  const close = () => {
    context.close().catch(() => {
      process.exitCode = 1;
    });
  };

  process.once("SIGINT", close);
  process.once("SIGTERM", close);

  try {
    const owner = { key: "owner-a", displayName: "所有者A" } as const;
    await authenticateTestSession(context, owner);
    await assertCookieIsHostOnly(context, owner);
    const page = context.pages()[0];
    await page.goto(`${origin}/login`);

    for (const format of ["markdown", "html"] as const) {
      const extension = format === "markdown" ? "md" : "html";

      const source = await readFile(
        new URL(`../fixtures/files/${format}/sample.${extension}`, import.meta.url),
        "utf8",
      );

      // ブラウザから既存fixture APIへ接続し、DB/MinIOへ実データを作る。
      const draftId = await page.evaluate(
        async (input) => {
          const response = await fetch("/api/v1/__e2e/fixtures", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ owner: "a", ...input }),
          });

          if (response.status !== 201) throw new Error(`fixture作成失敗: ${response.status}`);
          const body: unknown = await response.json();
          if (
            !body ||
            typeof body !== "object" ||
            !("draftId" in body) ||
            typeof body.draftId !== "string" ||
            !/^[0-9a-f-]{36}$/.test(body.draftId)
          )
            throw new Error("fixture応答にdraft IDがありません。");
          return body.draftId;
        },
        { format, source },
      );

      const editor = await context.newPage();
      const url = `${origin}/knowledge-drafts/${draftId}/edit?mode=editor`;
      await editor.goto(url);
      await expect(editor.locator("#knowledge-source")).toHaveValue(source, { timeout: 15_000 });
      console.log(`${format}: ${url}`);

      if (check) {
        await editor.getByRole("button", { name: "正式保存", exact: true }).click();
        await expect(editor).toHaveURL(/\/knowledge\/[0-9a-f-]+\/edit(?:\?.*)?$/, {
          timeout: 15_000,
        });
        await editor.goto(`${editor.url().split("?")[0]}?mode=editor`);
        await expect(editor.locator("#knowledge-source")).toHaveValue(source, { timeout: 15_000 });
        if (format === "html") {
          await editor.getByRole("button", { name: "プレビュー", exact: true }).click();
          await expect(editor.frameLocator("iframe").locator("h1")).toHaveText("E2E HTML");
        }
      }
    }
    await page.close();
    if (check) {
      console.log("手動環境: TLS・認証fixture・2形式の正式保存/再読込・HTML隔離表示 PASS");
    } else {
      console.log(
        "正式保存・編集を試せます。ブラウザを閉じるとこのコマンドが終了します。環境停止: task manual:down",
      );
      await context.waitForEvent("close", { timeout: 0 });
    }
  } finally {
    process.removeListener("SIGINT", close);
    process.removeListener("SIGTERM", close);
    await context.close();
  }
} finally {
  await rm(profile, { recursive: true, force: true, maxRetries: 5 });
}
