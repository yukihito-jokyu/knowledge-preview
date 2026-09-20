import type { APIRequestContext } from "@playwright/test";
import { expect, test } from "../../fixtures/test";
import { authenticatedHeaders, authenticateTestSession } from "../../fixtures/auth/test-session";
import { prepareFixture } from "../../fixtures/knowledge/prepare";
import type { FixtureUser } from "../../fixtures/auth/types";

const ownerA: FixtureUser = { key: "owner-a", displayName: "所有者A" };
const headers = authenticatedHeaders(ownerA);

async function commitFixture(request: APIRequestContext, source: string): Promise<string> {
  const draft = await prepareFixture(request, { owner: "a", format: "markdown", source });

  const response = await request.post(`/api/v1/knowledge-drafts/${draft.draftId}/commit`, {
    headers,
    data: { version: 1, source },
  });

  expect(response.status()).toBe(201);
  const body = (await response.json()) as { knowledgeId?: unknown };
  if (typeof body.knowledgeId !== "string") throw new Error("E2E commit did not return an id");
  return body.knowledgeId;
}

async function createFolder(request: APIRequestContext, name: string, parentId?: string) {
  const response = await request.post("/api/v1/folders", {
    headers,
    data: { name, ...(parentId ? { parentId } : {}) },
  });

  expect(response.status()).toBe(201);
  return (await response.json()) as {
    id: string;
    name: string;
    parentId: string | null;
    version: number;
  };
}

// Issue #15 X02/X03：owner Aの26件を実DBへ保存し、一覧の安定paginationとURL条件を確認する。
test("26件の知識を一覧でき、paginationとtitle・tag・本文検索のURL条件が再現される", async ({
  page,
  request,
}, testInfo) => {
  const marker = `pagination${testInfo.project.name}`;
  const title = `searchtitle${testInfo.project.name}`;
  const tagA = `searchtag-a-${testInfo.project.name}`;
  const tagB = `searchtag-b-${testInfo.project.name}`;
  const bothTitle = `searchboth${testInfo.project.name}`;
  const bothBody = `searchbothbody${testInfo.project.name}`;
  const body = `searchbody${testInfo.project.name}`;

  const sources = Array.from({ length: 26 }, (_, index) => {
    if (index === 0) return `---\ntitle: ${title}\ntags:\n  - ${tagA}\n---\n\n${body}\n${marker}`;
    if (index === 1)
      return `---\ntitle: tagBonly${testInfo.project.name}\ntags:\n  - ${tagB}\n---\n\nbodyBonly${testInfo.project.name}\n${marker}`;
    if (index === 2)
      return `---\ntitle: ${bothTitle}\ntags:\n  - ${tagA}\n  - ${tagB}\n---\n\n${bothBody}\n${marker}`;
    return `---\ntitle: ${marker}item${index + 1}\n---\n\n${marker}body${index + 1}\n${marker}`;
  });

  await Promise.all(sources.map((source) => commitFixture(request, source)));

  const listResponse = await request.get(
    `/api/v1/knowledge?query=${encodeURIComponent(marker)}&page=1&pageSize=25`,
    { headers },
  );

  expect(listResponse.status()).toBe(200);
  await expect(listResponse.json()).resolves.toMatchObject({ total: 26 });

  await authenticateTestSession(page.context(), ownerA);
  await page.goto(`/knowledge?q=${marker}&page=1&pageSize=25`);
  await expect(page.getByText("26件中 1〜25件", { exact: true })).toBeVisible({
    timeout: 15_000,
  });
  await expect(page.getByRole("button", { name: "次へ", exact: true })).toBeEnabled();
  await page.getByRole("button", { name: "次へ", exact: true }).click();
  await expect(page).toHaveURL(/page=2/, { timeout: 15_000 });
  await expect(page.getByText("26件中 26〜26件", { exact: true })).toBeVisible({
    timeout: 15_000,
  });
  await page.getByRole("button", { name: "前へ", exact: true }).click();

  const search = page.locator("#knowledge-search");
  await search.fill(title);

  const titleSearch = page.waitForResponse((response) =>
    response.url().includes(`/api/v1/knowledge?query=${title}`),
  );

  await page.getByRole("button", { name: "検索", exact: true }).click();
  expect((await titleSearch).status()).toBe(200);
  await expect(page).toHaveURL((url) => url.searchParams.get("q") === title);
  await expect(page.getByText("1件中 1〜1件", { exact: true })).toBeVisible();
  await expect(page.getByRole("link", { name: title, exact: true })).toBeVisible({
    timeout: 15_000,
  });
  await expect(page.getByRole("link", { name: bothTitle, exact: true })).toHaveCount(0);

  await page.goto(`/knowledge?q=${encodeURIComponent(marker)}&page=1&pageSize=25`);
  await expect(page.getByText("26件中 1〜25件", { exact: true })).toBeVisible();

  const tagAResponse = page.waitForResponse((response) => {
    if (!response.url().includes("/api/v1/knowledge?")) return false;
    const url = new URL(response.url());
    return response.request().method() === "GET" && url.searchParams.getAll("tag").includes(tagA);
  });

  await page
    .getByRole("link", { name: `#${tagA}`, exact: true })
    .first()
    .click();
  expect((await tagAResponse).status()).toBe(200);
  await expect(page).toHaveURL((url) => url.searchParams.get("tag") === JSON.stringify([tagA]));
  await expect(page.getByText("2件中 1〜2件", { exact: true })).toBeVisible();
  await expect(page.getByRole("link", { name: title, exact: true })).toBeVisible();
  await expect(page.getByRole("link", { name: bothTitle, exact: true })).toBeVisible();
  await expect(
    page.getByRole("link", { name: `tagBonly${testInfo.project.name}`, exact: true }),
  ).toHaveCount(0);
  await expect(page.getByRole("link", { name: `${marker}item4`, exact: true })).toHaveCount(0);

  await page.getByRole("link", { name: `#${tagB}`, exact: true }).click();
  await expect(page).toHaveURL(
    (url) => url.searchParams.get("tag") === JSON.stringify([tagA, tagB]),
  );
  await expect(page.getByText("1件中 1〜1件", { exact: true })).toBeVisible();
  await expect(page.getByRole("link", { name: bothTitle, exact: true })).toBeVisible();
  await expect(page.getByRole("link", { name: title, exact: true })).toHaveCount(0);

  await page.goto(
    `/knowledge?q=${encodeURIComponent(marker)}&tag=${encodeURIComponent(tagA)}&tag=${encodeURIComponent(tagB)}&page=1&pageSize=25`,
  );
  await expect(page.getByText("1件中 1〜1件", { exact: true })).toBeVisible();
  await expect(page.getByRole("link", { name: bothTitle, exact: true })).toBeVisible();

  await page.goto(`/knowledge?q=${encodeURIComponent(body)}&page=1&pageSize=25`);
  await expect(page.getByText("1件中 1〜1件", { exact: true })).toBeVisible();
  await expect(page.getByRole("link", { name: title, exact: true })).toBeVisible({
    timeout: 15_000,
  });
  await expect(page.getByRole("link", { name: bothTitle, exact: true })).toHaveCount(0);
  await expect(
    page.getByRole("link", { name: `tagBonly${testInfo.project.name}`, exact: true }),
  ).toHaveCount(0);

  await page.goto(`/knowledge?q=${"あ".repeat(200)}&page=1&pageSize=25`);
  await expect(page).toHaveURL((url) => url.searchParams.get("q") === "あ".repeat(200));
  await search.fill("あ".repeat(200));
  await page.getByRole("button", { name: "検索", exact: true }).click();
  await expect(page).toHaveURL((url) => url.searchParams.get("q") === "あ".repeat(200));
  await search.fill("い".repeat(201));
  await page.getByRole("button", { name: "検索", exact: true }).click();
  await expect(page.getByRole("alert")).toContainText("200文字以内");
  await expect(page).toHaveURL((url) => url.searchParams.get("q") === "あ".repeat(200));
});

// Issue #15 X03：folderの親子、知識移動、rename/deleteとdialogのキーボードfocusを確認する。
test("親子folderへ知識を移動・改名し、非空拒否後にキーボードで削除できる", async ({
  page,
  request,
}, testInfo) => {
  const suffix = testInfo.project.name;
  const parentName = `E2E親-${suffix}`;
  const childName = `E2E子-${suffix}`;
  const parent = await createFolder(request, parentName);
  const child = await createFolder(request, childName, parent.id);

  const knowledgeId = await commitFixture(
    request,
    `---\ntitle: folder移動対象${suffix}\ntags:\n  - folder\n---\n\nfolder本文`,
  );

  await authenticateTestSession(page.context(), ownerA);
  await page.goto("/knowledge");
  await expect(page.getByRole("button", { name: parentName, exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: childName, exact: true })).toBeVisible();

  const itemTitle = `folder移動対象${suffix}`;
  const folderSelect = page.getByRole("combobox", { name: `${itemTitle}のフォルダ` });

  const moveResponse = page.waitForResponse(
    (response) =>
      response.url().includes(`/api/v1/knowledge/${knowledgeId}/folder`) &&
      response.request().method() === "PUT",
  );

  await folderSelect.selectOption(child.id);
  expect((await moveResponse).status()).toBe(200);
  await page.getByRole("button", { name: childName, exact: true }).click();
  await expect(page.getByRole("link", { name: itemTitle, exact: true })).toBeVisible();

  await page.getByRole("button", { name: `${parentName}を変更`, exact: true }).click();
  const folderName = page.getByRole("textbox", { name: "フォルダ名", exact: true });
  const renamedParent = `${parentName}改名`;
  await folderName.fill(renamedParent);

  const renameResponse = page.waitForResponse(
    (response) =>
      response.url().includes(`/api/v1/folders/${parent.id}`) &&
      response.request().method() === "PATCH",
  );

  await page.getByRole("dialog").getByRole("button", { name: "保存", exact: true }).click();
  expect((await renameResponse).status()).toBe(200);
  await expect(page.getByRole("button", { name: renamedParent, exact: true })).toBeVisible();

  const createButton = page.getByRole("button", { name: "追加", exact: true });
  await createButton.click();
  await expect(page.getByRole("dialog")).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(createButton).toBeFocused();

  await page.getByRole("button", { name: `${childName}を削除`, exact: true }).click();

  const nonEmptyDelete = page.waitForResponse(
    (response) =>
      response.url().includes(`/api/v1/folders/${child.id}`) &&
      response.request().method() === "DELETE",
  );

  await page.getByRole("alertdialog").getByRole("button", { name: "削除", exact: true }).click();
  expect((await nonEmptyDelete).status()).toBe(409);
  await expect(page.getByRole("alertdialog").getByRole("alert")).toContainText("中身がある");
  await page
    .getByRole("alertdialog")
    .getByRole("button", { name: "キャンセル", exact: true })
    .click();

  const moveBack = page.waitForResponse(
    (response) =>
      response.url().includes(`/api/v1/knowledge/${knowledgeId}/folder`) &&
      response.request().method() === "PUT",
  );

  await folderSelect.selectOption("");
  expect((await moveBack).status()).toBe(200);

  await page.getByRole("button", { name: `${childName}を削除`, exact: true }).click();

  const deleteChild = page.waitForResponse(
    (response) =>
      response.url().includes(`/api/v1/folders/${child.id}`) &&
      response.request().method() === "DELETE",
  );

  await page.getByRole("alertdialog").getByRole("button", { name: "削除", exact: true }).click();
  expect((await deleteChild).status()).toBe(204);

  await page.getByRole("button", { name: `${renamedParent}を削除`, exact: true }).click();

  const deleteParent = page.waitForResponse(
    (response) =>
      response.url().includes(`/api/v1/folders/${parent.id}`) &&
      response.request().method() === "DELETE",
  );

  await page.getByRole("alertdialog").getByRole("button", { name: "削除", exact: true }).click();
  expect((await deleteParent).status()).toBe(204);
});
