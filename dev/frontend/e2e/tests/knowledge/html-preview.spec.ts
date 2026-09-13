import { expect, test } from "../../fixtures/test";
import { authenticateTestSession } from "../../fixtures/auth/test-session";
import { prepareFixture } from "../../fixtures/knowledge/prepare";
import type { FixtureUser } from "../../fixtures/auth/types";

const ownerA: FixtureUser = { key: "owner-a", displayName: "所有者A" };

async function createCommitted(
  request: Parameters<typeof prepareFixture>[0],
  source: string,
): Promise<string> {
  const fixture = await prepareFixture(request, { owner: "a", format: "html", source });

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

// Issue #14 シナリオ4・6：保存成功後だけHTML iframeを更新し、危険な操作と親画面変更を許可しない。
test("HTMLを未保存のまま表示すると旧版を保ち、保存後は安全化された新版を表示する", async ({
  page,
  request,
}) => {
  const oldSource = "<h1>旧HTML</h1><p>保存済みの本文</p>";
  const knowledgeId = await createCommitted(request, oldSource);
  await authenticateTestSession(page.context(), ownerA);
  const externalRequests: string[] = [];
  page.on("request", (requestEvent) => {
    if (requestEvent.url().includes("evil.example")) externalRequests.push(requestEvent.url());
  });

  await page.goto(`/knowledge/${knowledgeId}/edit`);
  const iframe = page.frameLocator('iframe[title="保存済みHTMLプレビュー"]');
  await expect(iframe.locator("body")).toContainText("旧HTML");
  const parentHeading = page.getByRole("heading", { name: "E2E fixture", exact: true });
  await expect(parentHeading).toBeVisible();

  await page.getByRole("button", { name: "エディター", exact: true }).click();

  const newSource =
    '<h1>新版HTML</h1><script src="https://evil.example/steal.js"></script><button onclick="window.top.location=\'https://evil.example\'">操作</button>';

  await page.locator("#knowledge-source").fill(newSource);
  await page.getByRole("button", { name: "プレビュー", exact: true }).click();
  await expect(iframe.locator("body")).toContainText("旧HTML");

  await page.getByRole("button", { name: "エディター", exact: true }).click();

  const saveResponse = page.waitForResponse(
    (response) =>
      response.url().includes(`/api/v1/knowledge/${knowledgeId}`) &&
      response.request().method() === "PUT",
  );

  await page.getByRole("button", { name: "保存", exact: true }).click();
  expect((await saveResponse).status()).toBe(200);
  await expect(page.getByRole("status")).toContainText(
    "安全のためHTMLの一部を除去して表示しています。",
  );
  await page.getByRole("button", { name: "プレビュー", exact: true }).click();
  await expect(iframe.locator("body")).toContainText("新版HTML");
  await expect(iframe.locator("script")).toHaveCount(0);
  await expect(iframe.locator("[onclick]")).toHaveCount(0);
  await expect(parentHeading).toBeVisible();
  expect(externalRequests).toEqual([]);
});
