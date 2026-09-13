import { expect, test } from "@playwright/test";
import { authenticatedHeaders } from "../../fixtures/auth/test-session";
import { prepareFixture, type FixtureInput } from "../../fixtures/knowledge/prepare";
import type { FixtureUser } from "../../fixtures/auth/types";

const ownerA: FixtureUser = { key: "owner-a", displayName: "所有者A" };
const ownerB: FixtureUser = { key: "other-b", displayName: "別ユーザーB" };
const appOrigin = process.env.E2E_APP_ORIGIN ?? "https://app.knowledge.test";
const previewOrigin = process.env.E2E_PREVIEW_ORIGIN ?? "https://preview.knowledge.test";
const validSource = "# E2E\n\n保存される本文です。";

async function createDraft(
  request: Parameters<typeof prepareFixture>[0],
  input: FixtureInput = { owner: "a", format: "markdown", source: validSource },
) {
  return (await prepareFixture(request, input)).draftId;
}

async function commitDraft(
  request: Parameters<typeof prepareFixture>[0],
  draftId: string,
  source = validSource,
) {
  const response = await request.post(`/api/v1/knowledge-drafts/${draftId}/commit`, {
    headers: authenticatedHeaders(ownerA),
    data: { version: 1, source },
  });

  return response;
}

async function jsonBody(
  response: Awaited<ReturnType<Parameters<typeof prepareFixture>[0]["get"]>>,
) {
  return (await response.json()) as Record<string, unknown>;
}

// Issue #14 シナリオ1・2：fixtureから正式保存し、実DB/S3の再取得と最近の結果を照合する。
test("Markdownを正式保存して再取得すると、本文とメタデータと最近の表示が一致する", async ({
  request,
}) => {
  const source = `---\ntitle: E2E Markdown\ntags:\n  - integration\nlearningStatus: learning\n---\n\n${validSource}`;
  const draftId = await createDraft(request, { owner: "a", format: "markdown", source });
  const commit = await commitDraft(request, draftId, source);
  expect(commit.status()).toBe(201);
  const committed = (await commit.json()) as { knowledgeId?: unknown };
  expect(typeof committed.knowledgeId).toBe("string");
  const knowledgeId = committed.knowledgeId as string;

  const saved = await request.get(`/api/v1/knowledge/${knowledgeId}`, {
    headers: authenticatedHeaders(ownerA),
  });

  expect(saved.status()).toBe(200);
  await expect(saved.json()).resolves.toMatchObject({
    id: knowledgeId,
    title: "E2E Markdown",
    format: "markdown",
    source,
    tags: ["integration"],
    learningStatus: "learning",
    version: 1,
    visibility: "private",
  });

  const updatedSource = `${source}\n\n更新後の本文です。`;

  const update = await request.put(`/api/v1/knowledge/${knowledgeId}`, {
    headers: authenticatedHeaders(ownerA),
    data: { version: 1, source: updatedSource },
  });

  expect(update.status()).toBe(200);
  await expect(update.json()).resolves.toMatchObject({
    id: knowledgeId,
    source: updatedSource,
    version: 2,
  });

  const recent = await request.get("/api/v1/knowledge/recent", {
    headers: authenticatedHeaders(ownerA),
  });

  expect(recent.status()).toBe(200);
  const recentBody = await jsonBody(recent);
  expect(recentBody.items).toEqual(
    expect.arrayContaining([
      expect.objectContaining({ id: knowledgeId, title: "E2E Markdown", format: "markdown" }),
    ]),
  );
});

// Issue #14 シナリオ2：古いdraftを保持し、同内容の再送を同じIDへ収束させる。
test("10年前のdraftを正式保存すると、再送は同じIDになり別所有者からは見えない", async ({
  request,
}) => {
  const draftId = await createDraft(request, {
    owner: "a",
    format: "markdown",
    source: validSource,
    ageYears: 10,
  });

  const first = await commitDraft(request, draftId);
  expect(first.status()).toBe(201);
  const firstBody = (await first.json()) as { knowledgeId?: unknown };
  if (typeof firstBody.knowledgeId !== "string") throw new Error("E2E commit did not return an id");
  const knowledgeId = firstBody.knowledgeId;

  const retry = await commitDraft(request, draftId);
  expect(retry.status()).toBe(200);
  await expect(retry.json()).resolves.toEqual({
    knowledgeId,
    editPath: `/knowledge/${knowledgeId}/edit`,
  });

  const different = await commitDraft(request, draftId, `${validSource}\n別内容`);
  expect(different.status()).toBe(409);

  const other = await request.get(`/api/v1/knowledge-drafts/${draftId}`, {
    headers: authenticatedHeaders(ownerB),
  });

  expect(other.status()).toBe(404);

  const committedDraft = await request.get(`/api/v1/knowledge-drafts/${draftId}`, {
    headers: authenticatedHeaders(ownerA),
  });

  expect(committedDraft.status()).toBe(200);
  await expect(committedDraft.json()).resolves.toEqual({
    draftId,
    status: "committed",
    knowledgeId,
    editPath: `/knowledge/${knowledgeId}/edit`,
  });
});

// Issue #14 シナリオ3：同じ版を保存すると、先行更新だけが永続化される。
test("同じversionを2回保存すると、後続の入力は409になり先行本文だけが残る", async ({ request }) => {
  const draftId = await createDraft(request);
  const commit = await commitDraft(request, draftId);
  expect(commit.status()).toBe(201);
  const knowledgeId = ((await commit.json()) as { knowledgeId: string }).knowledgeId;

  const firstSource = `${validSource}\n先行更新`;
  const secondSource = `${validSource}\n後続入力`;

  const first = await request.put(`/api/v1/knowledge/${knowledgeId}`, {
    headers: authenticatedHeaders(ownerA),
    data: { version: 1, source: firstSource },
  });

  expect(first.status()).toBe(200);

  const second = await request.put(`/api/v1/knowledge/${knowledgeId}`, {
    headers: authenticatedHeaders(ownerA),
    data: { version: 1, source: secondSource },
  });

  expect(second.status()).toBe(409);

  const saved = await request.get(`/api/v1/knowledge/${knowledgeId}`, {
    headers: authenticatedHeaders(ownerA),
  });

  expect(saved.status()).toBe(200);
  await expect(saved.json()).resolves.toMatchObject({ source: firstSource, version: 2 });
});

// Issue #14 シナリオ4・7：HTMLは安全化済みの実ファイルを返し、旧版URLを無効にする。
test("HTMLを保存すると危険な要素が除去され、保存前のpreviewと旧版URLは使えない", async ({
  request,
}) => {
  const oldSource = "<h1>旧版</h1><p>本文</p>";
  const draftId = await createDraft(request, { owner: "a", format: "html", source: oldSource });
  const commit = await commitDraft(request, draftId, oldSource);
  expect(commit.status()).toBe(201);
  const knowledgeId = ((await commit.json()) as { knowledgeId: string }).knowledgeId;

  const ticketResponse = await request.post(`/api/v1/knowledge/${knowledgeId}/preview-ticket`, {
    headers: authenticatedHeaders(ownerA),
    data: { version: 1 },
  });

  expect(ticketResponse.status()).toBe(200);
  const oldTicket = ((await ticketResponse.json()) as { url?: unknown }).url;
  if (typeof oldTicket !== "string") throw new Error("E2E preview ticket did not return a URL");

  const newSource =
    '<h1>新版</h1><script>window.__e2e = true</script><button onclick="alert(1)">操作</button>';

  const update = await request.put(`/api/v1/knowledge/${knowledgeId}`, {
    headers: authenticatedHeaders(ownerA),
    data: { version: 1, source: newSource },
  });

  expect(update.status()).toBe(200);
  await expect(update.json()).resolves.toMatchObject({
    version: 2,
    source: newSource,
    warnings: [{ code: "html_sanitized" }],
  });

  const oldPreview = await request.get(oldTicket);
  expect(oldPreview.status()).toBe(404);

  const newTicketResponse = await request.post(`/api/v1/knowledge/${knowledgeId}/preview-ticket`, {
    headers: authenticatedHeaders(ownerA),
    data: { version: 2 },
  });

  expect(newTicketResponse.status()).toBe(200);
  const newTicket = ((await newTicketResponse.json()) as { url?: unknown }).url;
  if (typeof newTicket !== "string") throw new Error("E2E preview ticket did not return a URL");
  const preview = await request.get(newTicket);
  expect(preview.status()).toBe(200);
  const content = await preview.text();
  expect(content).toContain("新版");
  expect(content).not.toContain("<script");
  expect(content).not.toContain("onclick");
  expect(preview.headers()["content-type"]).toContain("text/html");
  expect(preview.headers()["cache-control"]).toBe("no-store");
  expect(preview.headers()["referrer-policy"]).toBe("no-referrer");
  expect(preview.headers()["x-content-type-options"]).toBe("nosniff");
  expect(preview.headers()["content-security-policy"]).toContain("script-src 'none'");
});

// Issue #14 シナリオ5：公開IDとアプリ側のpublicUrlは停止・再公開でも維持する。
test("公開HTMLは未認証で読め、停止後に404となり再公開で同じpublicUrlへ戻る", async ({
  request,
}) => {
  const source = "<h1>公開する本文</h1>";
  const draftId = await createDraft(request, { owner: "a", format: "html", source });
  const commit = await commitDraft(request, draftId, source);
  expect(commit.status()).toBe(201);
  const knowledgeId = ((await commit.json()) as { knowledgeId: string }).knowledgeId;

  const publish = await request.put(`/api/v1/knowledge/${knowledgeId}/visibility`, {
    headers: authenticatedHeaders(ownerA),
    data: { version: 1, visibility: "unlisted" },
  });

  expect(publish.status()).toBe(200);

  const published = (await publish.json()) as {
    publicId?: unknown;
    publicUrl?: unknown;
    version?: unknown;
  };

  expect(typeof published.publicId).toBe("string");
  if (typeof published.publicId !== "string")
    throw new Error("E2E publish did not return a public id");
  expect(published.publicUrl).toBe(`${appOrigin}/public/knowledge/${published.publicId}`);
  expect(published.version).toBe(2);
  const publicId = published.publicId as string;

  const firstPublic = await request.get(`${previewOrigin}/public/${publicId}/html?version=2`);
  expect(firstPublic.status()).toBe(200);
  await expect(firstPublic.text()).resolves.toContain("公開する本文");

  const stop = await request.put(`/api/v1/knowledge/${knowledgeId}/visibility`, {
    headers: authenticatedHeaders(ownerA),
    data: { version: 2, visibility: "private" },
  });

  expect(stop.status()).toBe(200);

  const stopped = (await stop.json()) as {
    publicId?: unknown;
    publicUrl?: unknown;
    version?: unknown;
  };

  expect(stopped.publicId).toBe(publicId);
  expect(stopped.publicUrl).toBe(`${appOrigin}/public/knowledge/${publicId}`);
  expect(stopped.version).toBe(3);
  expect((await request.get(`${previewOrigin}/public/${publicId}/html?version=2`)).status()).toBe(
    404,
  );

  const republish = await request.put(`/api/v1/knowledge/${knowledgeId}/visibility`, {
    headers: authenticatedHeaders(ownerA),
    data: { version: 3, visibility: "unlisted" },
  });

  expect(republish.status()).toBe(200);

  const reopened = (await republish.json()) as {
    publicId?: unknown;
    publicUrl?: unknown;
    version?: unknown;
  };

  expect(reopened.publicId).toBe(publicId);
  expect(reopened.publicUrl).toBe(`${appOrigin}/public/knowledge/${publicId}`);
  expect(reopened.version).toBe(4);
  expect((await request.get(`${previewOrigin}/public/${publicId}/html?version=4`)).status()).toBe(
    200,
  );
});

// Issue #14 シナリオ8：入力違反は保存せず、UTF-8 10 MiBを超える本文は413になる。
test("空白本文は422になり、10 MiB本文は保存できて超過本文は413になる", async ({ request }) => {
  const draftId = await createDraft(request);
  const empty = await commitDraft(request, draftId, " \n\t");
  expect(empty.status()).toBe(422);
  await expect(empty.json()).resolves.toMatchObject({ error: { code: "validation_failed" } });

  const boundarySource = "x".repeat(10 * 1024 * 1024);
  const boundaryDraft = await createDraft(request);
  const boundary = await commitDraft(request, boundaryDraft, boundarySource);
  expect(boundary.status()).toBe(201);
  const tooLargeDraft = await createDraft(request);
  const tooLarge = await commitDraft(request, tooLargeDraft, `${boundarySource}x`);
  expect(tooLarge.status()).toBe(413);
  await expect(tooLarge.json()).resolves.toMatchObject({ error: { code: "payload_too_large" } });
});

// Issue #14 シナリオ10：所有者境界とOrigin保護を実APIで確認する。
test("別所有者の知識は404になり、許可されないOriginからの更新は403になる", async ({ request }) => {
  const draftId = await createDraft(request);
  const commit = await commitDraft(request, draftId);
  expect(commit.status()).toBe(201);
  const knowledgeId = ((await commit.json()) as { knowledgeId: string }).knowledgeId;

  const other = await request.get(`/api/v1/knowledge/${knowledgeId}`, {
    headers: authenticatedHeaders(ownerB),
  });

  expect(other.status()).toBe(404);

  const forbidden = await request.put(`/api/v1/knowledge/${knowledgeId}`, {
    headers: { ...authenticatedHeaders(ownerA), Origin: "https://evil.example" },
    data: { version: 1, source: `${validSource}\n拒否される更新` },
  });

  expect(forbidden.status()).toBe(403);

  const saved = await request.get(`/api/v1/knowledge/${knowledgeId}`, {
    headers: authenticatedHeaders(ownerA),
  });

  expect(saved.status()).toBe(200);
  await expect(saved.json()).resolves.toMatchObject({ version: 1, source: validSource });
});
