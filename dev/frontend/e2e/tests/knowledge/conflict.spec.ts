import { expect, test } from "../../fixtures/test";
import { authenticateTestSession } from "../../fixtures/auth/test-session";
import { prepareFixture } from "../../fixtures/knowledge/prepare";
import type { FixtureUser } from "../../fixtures/auth/types";

const ownerA: FixtureUser = { key: "owner-a", displayName: "所有者A" };
const validSource = "# E2E\n\nブラウザから保存する本文です。";

// Issue #14 シナリオ3：二つの編集タブで同じ版を送信し、409でも後続入力を保持する。
test("同じ版を二つの編集タブから保存すると、後続タブは409を表示して入力を保持する", async ({
  page,
  request,
  context,
}) => {
  test.setTimeout(60_000);

  const fixture = await prepareFixture(request, {
    owner: "a",
    format: "markdown",
    source: validSource,
  });

  const commit = await request.post(`/api/v1/knowledge-drafts/${fixture.draftId}/commit`, {
    headers: {
      Cookie: "__Host-kp_e2e_actor=a",
      Origin: "https://app.knowledge.test",
    },
    data: { version: 1, source: validSource },
  });

  expect(commit.status()).toBe(201);
  const knowledgeId = ((await commit.json()) as { knowledgeId: string }).knowledgeId;

  await authenticateTestSession(context, ownerA);
  await page.goto(`/knowledge/${knowledgeId}/edit?mode=editor`);
  await expect(page.locator("#knowledge-source")).toHaveValue(validSource, { timeout: 15_000 });

  const secondPage = await context.newPage();
  try {
    await secondPage.goto(`/knowledge/${knowledgeId}/edit?mode=editor`);
    await expect(secondPage.locator("#knowledge-source")).toHaveValue(validSource, {
      timeout: 15_000,
    });
    // 二つのタブを同じ版から編集し、操作するタブを前面化して保存競合を検証する。
    await page.bringToFront();
    const firstSource = `${validSource}\n\n先行タブの保存。`;
    await page.locator("#knowledge-source").fill(firstSource);

    const [firstSave] = await Promise.all([
      page.waitForResponse(
        (response) =>
          response.url().includes(`/api/v1/knowledge/${knowledgeId}`) &&
          response.request().method() === "PUT",
      ),
      page.getByRole("button", { name: "保存", exact: true }).click(),
    ]);

    expect(firstSave.status()).toBe(200);
    const secondSource = `${validSource}\n\n後続タブの入力を維持。`;
    await secondPage.bringToFront();
    await secondPage.locator("#knowledge-source").fill(secondSource);

    const [secondSave] = await Promise.all([
      secondPage.waitForResponse(
        (response) =>
          response.url().includes(`/api/v1/knowledge/${knowledgeId}`) &&
          response.request().method() === "PUT",
      ),
      secondPage.getByRole("button", { name: "保存", exact: true }).click(),
    ]);

    expect(secondSave.status()).toBe(409);
    await expect(secondPage.getByRole("alert")).toContainText("別の操作で更新されています");
    await expect(secondPage.locator("#knowledge-source")).toHaveValue(secondSource);
  } finally {
    await secondPage.close();
  }
});
