import { expect, test } from "../../fixtures/test";

// Issue #14 シナリオ10：未認証で保護APIへアクセスすると、実APIが401の共通エラーを返す。
test("未認証で最近の知識を取得すると、実APIが認証要求を返す", async ({ request }) => {
  const response = await request.get("/api/v1/knowledge/recent");

  expect(response.status()).toBe(401);
  await expect(response.json()).resolves.toEqual({
    error: {
      code: "unauthenticated",
      message: "authentication is required",
    },
  });
});
