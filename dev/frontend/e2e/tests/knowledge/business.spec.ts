import { expect, test } from "../../fixtures/test";
import { assertCookieIsHostOnly, authenticateTestSession } from "../../fixtures/auth/test-session";
import { prepareFixture } from "../../fixtures/knowledge/prepare";
import type { FixtureUser } from "../../fixtures/auth/types";

const ownerA: FixtureUser = { key: "owner-a", displayName: "所有者A" };
const validSource = "# E2E\n\nブラウザから保存する本文です。";

async function createCommitted(
  request: Parameters<typeof prepareFixture>[0],
  format: "markdown" | "html",
  source: string,
): Promise<string> {
  const fixture = await prepareFixture(request, { owner: "a", format, source });

  const commit = await request.post(`/api/v1/knowledge-drafts/${fixture.draftId}/commit`, {
    headers: {
      Cookie: "__Host-kp_e2e_actor=a",
      Origin: "https://app.knowledge.test",
    },
    data: { version: 1, source },
  });

  expect(commit.status()).toBe(201);
  const body = (await commit.json()) as { knowledgeId?: unknown };
  if (typeof body.knowledgeId !== "string") throw new Error("E2E commit did not return an id");
  return body.knowledgeId;
}

// Issue #14 シナリオ1：認証だけを注入し、編集画面から実API・DB・S3へ保存する。
test("Markdownを編集して保存すると、再読込後も入力した本文と最近の表示が一致する", async ({
  page,
  request,
}) => {
  const source = `---\ntitle: ブラウザMarkdown\ntags:\n  - browser\nlearningStatus: learning\n---\n\n${validSource}`;
  const knowledgeId = await createCommitted(request, "markdown", source);
  await authenticateTestSession(page.context(), ownerA);
  await assertCookieIsHostOnly(page.context(), ownerA);

  await page.goto(`/knowledge/${knowledgeId}/edit?mode=editor`);
  await expect(page.getByRole("heading", { name: "ブラウザMarkdown", exact: true })).toBeVisible();
  const updated = `${source}\n\n再読込でも残る変更。`;
  await page.locator("#knowledge-source").fill(updated);

  const saveResponse = page.waitForResponse(
    (response) =>
      response.url().includes(`/api/v1/knowledge/${knowledgeId}`) &&
      response.request().method() === "PUT",
  );

  await page.getByRole("button", { name: "保存", exact: true }).click();
  expect((await saveResponse).status()).toBe(200);
  await expect(page.getByRole("status")).toContainText("保存しました。");

  await page.reload();
  await page.getByRole("button", { name: "エディター", exact: true }).click();
  await expect(page.locator("#knowledge-source")).toHaveValue(updated);
  await expect(page.getByRole("link", { name: "ブラウザMarkdown" }).first()).toBeVisible();
});
