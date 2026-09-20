import { open, mkdtemp, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import type { Page } from "@playwright/test";
import { expect, test } from "../../fixtures/test";
import { authenticateTestSession } from "../../fixtures/auth/test-session";
import { failNextUpload } from "../../fixtures/knowledge/test-controls";
import type { FixtureUser } from "../../fixtures/auth/types";

const ownerA: FixtureUser = { key: "owner-a", displayName: "所有者A" };
const markdownFile = resolve("e2e/fixtures/files/markdown/sample.md");
const htmlFile = resolve("e2e/fixtures/files/html/sample.html");

async function choose(page: Page, file: string) {
  const input = page.locator("#knowledge-file");
  await input.setInputFiles(file);
}

// Issue #15 X02：fixture認証を注入し、実multipart upload→draft編集→commit→一覧→再取得を確認する。
test("Markdownを選んで正式保存すると、一覧と再読込後の本文が一致する", async ({ page }) => {
  await authenticateTestSession(page.context(), ownerA);
  await page.goto("/knowledge/new");
  await expect(page.getByText("知識を作成", { exact: true })).toBeVisible();

  const uploadResponse = page.waitForResponse(
    (response) =>
      response.url().includes("/api/v1/knowledge-drafts") && response.request().method() === "POST",
  );

  await choose(page, markdownFile);
  expect((await uploadResponse).status()).toBe(201);
  await expect(page).toHaveURL(/\/knowledge-drafts\/[0-9a-f-]+\/edit(?:\?.*)?$/);

  await page.getByRole("button", { name: "エディター", exact: true }).click();
  const source = page.locator("#knowledge-source");
  await expect(source).toHaveValue(/E2E Markdown/);
  const updated = `${await source.inputValue()}\n\nuploadから正式保存した本文。`;
  await source.fill(updated);

  const commitResponse = page.waitForResponse(
    (response) =>
      response.url().includes("/api/v1/knowledge-drafts/") &&
      response.url().endsWith("/commit") &&
      response.request().method() === "POST",
  );

  await page.getByRole("button", { name: "正式保存", exact: true }).click();
  expect((await commitResponse).status()).toBe(201);
  await expect(page).toHaveURL(/\/knowledge\/[0-9a-f-]+\/edit(?:\?.*)?$/);
  const knowledgePath = new URL(page.url()).pathname;

  await page.getByRole("link", { name: "マイ辞書", exact: true }).click();
  await expect(page.getByRole("heading", { name: "知識一覧", exact: true })).toBeVisible();
  const savedLink = page.locator(`a[href="${knowledgePath}"]`);
  await expect(savedLink).toBeVisible();

  await savedLink.click();
  await expect(page).toHaveURL(/\/knowledge\/[0-9a-f-]+\/edit(?:\?.*)?$/);
  await page.getByRole("button", { name: "エディター", exact: true }).click();
  await expect(page.locator("#knowledge-source")).toHaveValue(updated);
});

// Issue #15 X02・X07：HTMLの入口から正式保存し、既存のsandbox previewへ接続する。
test("HTMLを選んで正式保存すると、安全化された隔離プレビューが表示される", async ({ page }) => {
  await authenticateTestSession(page.context(), ownerA);
  await page.goto("/knowledge/new");
  await choose(page, htmlFile);
  await expect(page).toHaveURL(/\/knowledge-drafts\/[0-9a-f-]+\/edit(?:\?.*)?$/);
  await page.getByRole("button", { name: "正式保存", exact: true }).click();
  await expect(page).toHaveURL(/\/knowledge\/[0-9a-f-]+\/edit(?:\?.*)?$/);

  const iframe = page.frameLocator('iframe[title="保存済みHTMLプレビュー"]');
  await expect(iframe.locator("h1")).toContainText("E2E HTML");
  const preview = page.locator('iframe[title="保存済みHTMLプレビュー"]');
  await expect(preview).toHaveAttribute("sandbox", "");
  await expect(preview).toHaveAttribute("referrerpolicy", "no-referrer");
});

// Issue #15 X02：実APIの解析失敗後、成功した別ファイルを同じ入力から再選択できることを確認する。
test("壊れたfront matterの失敗後に別ファイルを選ぶと、uploadを再試行できる", async ({ page }) => {
  await authenticateTestSession(page.context(), ownerA);
  await page.goto("/knowledge/new");
  const input = page.locator("#knowledge-file");
  await input.setInputFiles({
    name: "broken.md",
    mimeType: "text/markdown",
    buffer: Buffer.from("---\ntitle: [壊れた\n---\n\n本文"),
  });
  await expect(page.getByRole("alert")).toContainText("ファイルの内容を確認してください");
  await page.getByRole("button", { name: "別のファイルを選ぶ", exact: true }).click();
  await input.setInputFiles(markdownFile);
  await expect(page).toHaveURL(/\/knowledge-drafts\/[0-9a-f-]+\/edit(?:\?.*)?$/);
});

test("保存障害では画面遷移せず、別ファイルの選択でuploadを再試行できる", async ({
  page,
  request,
}) => {
  await authenticateTestSession(page.context(), ownerA);
  await failNextUpload(request, ownerA, "database");
  await page.goto("/knowledge/new");

  const uploadResponse = page.waitForResponse(
    (response) =>
      response.url().includes("/api/v1/knowledge-drafts") && response.request().method() === "POST",
  );

  await choose(page, markdownFile);
  expect((await uploadResponse).status()).toBe(503);
  await expect(page).toHaveURL(/\/knowledge\/new(?:\?.*)?$/);
  await expect(page.getByRole("alert")).toContainText("解析できませんでした");

  await page.getByRole("button", { name: "別のファイルを選ぶ", exact: true }).click();

  const retryResponse = page.waitForResponse(
    (response) =>
      response.url().includes("/api/v1/knowledge-drafts") && response.request().method() === "POST",
  );

  await choose(page, htmlFile);
  expect((await retryResponse).status()).toBe(201);
  await expect(page).toHaveURL(/\/knowledge-drafts\/[0-9a-f-]+\/edit(?:\?.*)?$/);
});

test("10 MiBを1 byte超えるファイルは送信せず、別ファイルはuploadできる", async ({ page }) => {
  await authenticateTestSession(page.context(), ownerA);
  await page.goto("/knowledge/new");

  let uploadRequests = 0;

  const onRequest = (request: { url: () => string; method: () => string }) => {
    if (request.url().includes("/api/v1/knowledge-drafts") && request.method() === "POST")
      uploadRequests++;
  };

  page.on("request", onRequest);
  const input = page.locator("#knowledge-file");
  const directory = await mkdtemp(join(tmpdir(), "knowledge-e2e-upload-"));
  const tooLargeFile = join(directory, "too-large.md");
  const file = await open(tooLargeFile, "w");
  try {
    await file.truncate(10 * 1024 * 1024 + 1);
    await file.close();
    await input.setInputFiles(tooLargeFile);

    await expect(page.getByRole("alert")).toContainText("10 MiB以内");
    expect(uploadRequests).toBe(0);

    await page.getByRole("button", { name: "別のファイルを選ぶ", exact: true }).click();
    await choose(page, markdownFile);
    await expect(page).toHaveURL(/\/knowledge-drafts\/[0-9a-f-]+\/edit(?:\?.*)?$/);
  } finally {
    await file.close().catch(() => undefined);
    await rm(directory, { recursive: true, force: true });
    page.off("request", onRequest);
  }
});
