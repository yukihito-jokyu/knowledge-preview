import type { APIRequestContext } from "@playwright/test";
import { expect, test } from "../../fixtures/test";
import { authenticatedHeaders } from "../../fixtures/auth/test-session";
import { prepareFixture } from "../../fixtures/knowledge/prepare";
import type { FixtureUser } from "../../fixtures/auth/types";

const ownerA: FixtureUser = { key: "owner-a", displayName: "所有者A" };
const ownerB: FixtureUser = { key: "other-b", displayName: "別ユーザーB" };
const headers = authenticatedHeaders(ownerA);

type Folder = { id: string; name: string; parentId: string | null; version: number };

async function createFolder(request: APIRequestContext, name: string, parentId?: string) {
  const response = await request.post("/api/v1/folders", {
    headers,
    data: { name, ...(parentId ? { parentId } : {}) },
  });

  expect(response.status()).toBe(201);
  return (await response.json()) as Folder;
}

async function createKnowledge(
  request: APIRequestContext,
): Promise<{ id: string; version: number }> {
  const source = "---\ntitle: folder契約\n---\n\n本文";
  const draft = await prepareFixture(request, { owner: "a", format: "markdown", source });

  const response = await request.post(`/api/v1/knowledge-drafts/${draft.draftId}/commit`, {
    headers,
    data: { version: 1, source },
  });

  expect(response.status()).toBe(201);
  const body = (await response.json()) as { knowledgeId: string };
  return { id: body.knowledgeId, version: 1 };
}

// Issue #15 X03：親子作成・rename・version競合・循環・非空削除・知識移動を実APIで確認する。
test("folderの親子関係とversion境界を実APIで維持し、循環と非空削除を拒否する", async ({
  request,
}, testInfo) => {
  const suffix = testInfo.project.name;
  const parent = await createFolder(request, `契約親-${suffix}`);
  const child = await createFolder(request, `契約子-${suffix}`, parent.id);

  const renamed = await request.patch(`/api/v1/folders/${parent.id}`, {
    headers,
    data: { name: `契約親改名-${suffix}`, version: parent.version },
  });

  expect(renamed.status()).toBe(200);
  await expect(renamed.json()).resolves.toMatchObject({
    id: parent.id,
    name: `契約親改名-${suffix}`,
    version: 2,
  });

  const staleRename = await request.patch(`/api/v1/folders/${parent.id}`, {
    headers,
    data: { name: `古い版-${suffix}`, version: parent.version },
  });

  expect(staleRename.status()).toBe(409);

  const cycle = await request.patch(`/api/v1/folders/${parent.id}`, {
    headers,
    data: { name: `契約親改名-${suffix}`, parentId: child.id, version: 2 },
  });

  expect(cycle.status()).toBe(422);
  await expect(cycle.json()).resolves.toMatchObject({
    error: { code: "validation_failed", details: [{ field: "parentId", reason: "cycle" }] },
  });

  const knowledge = await createKnowledge(request);

  const moved = await request.put(`/api/v1/knowledge/${knowledge.id}/folder`, {
    headers,
    data: { folderId: child.id, version: knowledge.version },
  });

  expect(moved.status()).toBe(200);
  await expect(moved.json()).resolves.toMatchObject({ folder: { id: child.id }, version: 2 });

  const staleMove = await request.put(`/api/v1/knowledge/${knowledge.id}/folder`, {
    headers,
    data: { folderId: null, version: knowledge.version },
  });

  expect(staleMove.status()).toBe(409);

  const nonEmpty = await request.delete(`/api/v1/folders/${child.id}?version=1`, { headers });
  expect(nonEmpty.status()).toBe(409);

  const parentNonEmpty = await request.delete(`/api/v1/folders/${parent.id}?version=2`, {
    headers,
  });

  expect(parentNonEmpty.status()).toBe(409);
});

// Issue #15 X03：folderとknowledgeの所有者境界を同一の404契約で確認する。
test("別所有者からfolderと知識を参照・変更できない", async ({ request }, testInfo) => {
  const folder = await createFolder(request, `所有者境界-${testInfo.project.name}`);
  const folders = await request.get("/api/v1/folders", { headers: authenticatedHeaders(ownerB) });
  expect(folders.status()).toBe(200);
  await expect(folders.json()).resolves.toEqual({ items: [] });

  const rename = await request.patch(`/api/v1/folders/${folder.id}`, {
    headers: authenticatedHeaders(ownerB),
    data: { name: "変更不可", version: 1 },
  });

  expect(rename.status()).toBe(404);

  const draftSource = "---\ntitle: owner boundary draft\n---\n\nAの下書き本文";

  const draft = await prepareFixture(request, {
    owner: "a",
    format: "markdown",
    source: draftSource,
  });

  const commitAsB = await request.post(`/api/v1/knowledge-drafts/${draft.draftId}/commit`, {
    headers: authenticatedHeaders(ownerB),
    data: { version: 1, source: draftSource },
  });

  expect(commitAsB.status()).toBe(404);

  const draftAsA = await request.get(`/api/v1/knowledge-drafts/${draft.draftId}`, {
    headers: authenticatedHeaders(ownerA),
  });

  expect(draftAsA.status()).toBe(200);
  await expect(draftAsA.json()).resolves.toMatchObject({
    status: "pending",
    source: draftSource,
    version: 1,
  });

  const commitAsA = await request.post(`/api/v1/knowledge-drafts/${draft.draftId}/commit`, {
    headers: authenticatedHeaders(ownerA),
    data: { version: 1, source: draftSource },
  });

  expect(commitAsA.status()).toBe(201);
  const knowledgeId = ((await commitAsA.json()) as { knowledgeId: string }).knowledgeId;

  const move = await request.put(`/api/v1/knowledge/${knowledgeId}/folder`, {
    headers,
    data: { folderId: folder.id, version: 1 },
  });

  expect(move.status()).toBe(200);

  const beforeBMove = await request.get(`/api/v1/knowledge/${knowledgeId}`, { headers });
  expect(beforeBMove.status()).toBe(200);

  const beforeBMoveBody = (await beforeBMove.json()) as {
    source: string;
    version: number;
    folder: { id: string } | null;
  };

  expect(beforeBMoveBody.folder?.id).toBe(folder.id);

  const moveAsB = await request.put(`/api/v1/knowledge/${knowledgeId}/folder`, {
    headers: authenticatedHeaders(ownerB),
    data: { folderId: null, version: beforeBMoveBody.version },
  });

  expect(moveAsB.status()).toBe(404);

  const afterBMove = await request.get(`/api/v1/knowledge/${knowledgeId}`, { headers });
  expect(afterBMove.status()).toBe(200);
  await expect(afterBMove.json()).resolves.toMatchObject(beforeBMoveBody);
});
