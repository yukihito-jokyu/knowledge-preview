import { expect, test } from "../../fixtures/test";
import { authenticateTestSession } from "../../fixtures/auth/test-session";
import { prepareFixture } from "../../fixtures/knowledge/prepare";
import type { FixtureUser } from "../../fixtures/auth/types";

const ownerA: FixtureUser = { key: "owner-a", displayName: "所有者A" };
const validSource = "# E2E\n\ndraftから正式保存する本文です。";

// Issue #14 シナリオ2：draftは実APIで正式保存し、保存済みIDへ遷移して再取得する。
test("draftを正式保存すると保存済み画面へ遷移し、再読込後に本文を取得できる", async ({
  page,
  request,
}) => {
  const draft = await prepareFixture(request, {
    owner: "a",
    format: "markdown",
    source: validSource,
    ageYears: 10,
  });

  await authenticateTestSession(page.context(), ownerA);
  await page.goto(`/knowledge-drafts/${draft.draftId}/edit?mode=editor`);
  await expect(page.locator("#knowledge-source")).toHaveValue(validSource);

  const updated = `${validSource}\n\ndraftから正式保存。`;
  await page.locator("#knowledge-source").fill(updated);

  const commitResponse = page.waitForResponse(
    (response) =>
      response.url().includes(`/api/v1/knowledge-drafts/${draft.draftId}/commit`) &&
      response.request().method() === "POST",
  );

  await page.getByRole("button", { name: "正式保存", exact: true }).click();
  expect((await commitResponse).status()).toBe(201);
  await expect(page).toHaveURL(/\/knowledge\/[0-9a-f-]+\/edit(?:\?.*)?$/);

  await page.reload({ waitUntil: "domcontentloaded" });
  const editorUrl = new URL(page.url());
  editorUrl.searchParams.set("mode", "editor");
  await page.goto(editorUrl.toString(), { waitUntil: "commit" });
  await expect(page.locator("#knowledge-source")).toHaveValue(updated);
});
