import { expect, test } from "../fixtures/test";

// Issue #7: 環境の疎通確認。ログイン・公開閲覧の業務シナリオの成功ではない。
test("未ログインでも、HTTPSでログイン画面を表示できる", async ({ page }) => {
  const response = await page.goto("/login");
  expect(response?.status()).toBe(200);
  await expect(page.getByRole("heading", { name: "ログイン", exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "GitHubでログイン", exact: true })).toBeVisible();
});

test("DBが利用可能なら、HTTPS経由の準備確認が成功する", async ({ request }) => {
  const response = await request.get("/api/ready");
  expect(response.status()).toBe(204);
});

test("未実装のOAuth認可へアクセスすると、未実装エラーが返る", async ({ request }) => {
  const response = await request.get("https://oauth.knowledge.test/authorize");
  expect(response.status()).toBe(501);
});

test("未実装の公開配信へアクセスすると、未実装エラーが返る", async ({ request }) => {
  const response = await request.get("https://preview.knowledge.test/");
  expect(response.status()).toBe(501);
});
