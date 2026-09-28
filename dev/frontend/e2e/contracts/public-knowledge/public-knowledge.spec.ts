import { expect, test } from "../../fixtures/test";
import { authenticatedHeaders } from "../../fixtures/auth/test-session";
import type { FixtureUser } from "../../fixtures/auth/types";
import { commitAndPublish } from "../../fixtures/knowledge/prepare";
import { clearPublicReadFault, failNextPublicRead } from "../../fixtures/knowledge/test-controls";

const ownerA: FixtureUser = { key: "owner-a", displayName: "所有者A" };
const publicPath = (publicId: string) => `/api/v1/public/knowledge/${publicId}`;

const markdownSource = `---
title: 公開契約記事
tags:
  - shared
  - contract
---

# 本文見出し

公開本文です。`;

async function getPublic(request: Parameters<typeof commitAndPublish>[0], publicId: string) {
  return request.get(publicPath(publicId));
}

// Issue #17 シナリオ1・3・5：公開専用DTO、現在版、ヘッダー、秘匿境界を実APIで確認する。
test("公開JSONは現在版だけをallow-listで返し、更新後の本文と関連記事を再取得できる", async ({
  request,
}) => {
  const published = await commitAndPublish(request, ownerA, {
    owner: "a",
    format: "markdown",
    source: markdownSource,
  });

  await commitAndPublish(request, ownerA, {
    owner: "a",
    format: "markdown",
    source: markdownSource.replace("公開契約記事", "関連記事").replace("本文見出し", "関連見出し"),
  });

  const first = await getPublic(request, published.publicId);
  expect(first.status()).toBe(200);
  expect(first.headers()["cache-control"]).toBe("no-store");
  expect(first.headers()["x-robots-tag"]).toBe("noindex, nofollow");
  const body = (await first.json()) as Record<string, unknown>;
  expect(Object.keys(body).sort()).toEqual([
    "content",
    "format",
    "publicId",
    "publicScope",
    "related",
    "summary",
    "tags",
    "title",
    "updatedAt",
    "version",
  ]);
  expect(body).toMatchObject({
    publicId: published.publicId,
    title: "公開契約記事",
    format: "markdown",
    tags: ["shared", "contract"],
    version: 2,
    publicScope: "unlisted",
  });
  expect(body.content).toEqual(
    expect.objectContaining({ markdown: expect.stringContaining("本文見出し") }),
  );
  expect(body).not.toHaveProperty("ownerId");
  expect(body).not.toHaveProperty("sourceKey");
  expect(body).not.toHaveProperty("htmlKey");
  expect(body).not.toHaveProperty("learningStatus");
  expect(body).not.toHaveProperty("folder");
  expect(body).not.toHaveProperty("draft");
  const related = body.related as Record<string, unknown>[];
  expect(related).toEqual(
    expect.arrayContaining([expect.objectContaining({ title: "関連記事", format: "markdown" })]),
  );
  for (const item of related) {
    expect(Object.keys(item).sort()).toEqual(["format", "publicId", "summary", "title"]);
  }

  const updatedSource = `${markdownSource.replace("公開契約記事", "公開契約記事・新版")}

新版の保存本文です。`;

  const update = await request.put(`/api/v1/knowledge/${published.knowledgeId}`, {
    headers: authenticatedHeaders(ownerA),
    data: { version: 2, source: updatedSource },
  });

  expect(update.status()).toBe(200);

  const current = await getPublic(request, published.publicId);
  expect(current.status()).toBe(200);
  await expect(current.json()).resolves.toMatchObject({
    title: "公開契約記事・新版",
    version: 3,
    content: { markdown: expect.stringContaining("新版の保存本文です") },
  });
});

// Issue #17 シナリオ4：不正・不存在・停止済みの公開IDは同じ404契約にする。
test("不正・不存在・停止済みの公開JSONは秘匿された404になる", async ({ request }) => {
  const invalid = await request.get(publicPath("invalid"));
  expect(invalid.status()).toBe(404);
  expect(invalid.headers()["cache-control"]).toBe("no-store");
  expect(invalid.headers()["x-robots-tag"]).toBe("noindex, nofollow");

  const missing = await request.get(publicPath("A".repeat(43)));
  expect(missing.status()).toBe(404);

  const published = await commitAndPublish(request, ownerA, {
    owner: "a",
    format: "markdown",
    source: "# 停止対象\n\n公開中の本文です。",
  });

  const stop = await request.put(`/api/v1/knowledge/${published.knowledgeId}/visibility`, {
    headers: authenticatedHeaders(ownerA),
    data: { version: published.version, visibility: "private" },
  });

  expect(stop.status()).toBe(200);

  const stopped = await getPublic(request, published.publicId);
  expect(stopped.status()).toBe(404);
  await expect(stopped.json()).resolves.toMatchObject({ error: { code: "not_found" } });
});

// Issue #17 シナリオ4：公開読取の実依存障害は503を返し、次の取得で回復する。
test("公開JSONの依存障害は503になり、再取得すると200へ回復する", async ({ request }) => {
  const published = await commitAndPublish(request, ownerA, {
    owner: "a",
    format: "markdown",
    source: "# 再試行対象\n\n障害後も公開される本文です。",
  });

  await clearPublicReadFault(request, ownerA, "public");
  try {
    await failNextPublicRead(request, ownerA, "public");

    const failed = await getPublic(request, published.publicId);
    expect(failed.status()).toBe(503);
    expect(failed.headers()["cache-control"]).toBe("no-store");
    await expect(failed.json()).resolves.toMatchObject({ error: { code: "unavailable" } });

    const recovered = await getPublic(request, published.publicId);
    expect(recovered.status()).toBe(200);
    await expect(recovered.json()).resolves.toMatchObject({ publicId: published.publicId });
  } finally {
    await clearPublicReadFault(request, ownerA, "public");
  }
});

// Issue #17 シナリオ4：公開object読取の実依存障害も503から回復する。
test("公開JSONのobject読取障害は503になり、再取得すると200へ回復する", async ({ request }) => {
  const published = await commitAndPublish(request, ownerA, {
    owner: "a",
    format: "markdown",
    source: "# object再試行対象\n\n障害後も公開される本文です。",
  });

  await clearPublicReadFault(request, ownerA, "get");
  try {
    await failNextPublicRead(request, ownerA, "get");

    const failed = await getPublic(request, published.publicId);
    expect(failed.status()).toBe(503);
    expect(failed.headers()["cache-control"]).toBe("no-store");
    await expect(failed.json()).resolves.toMatchObject({ error: { code: "unavailable" } });

    const recovered = await getPublic(request, published.publicId);
    expect(recovered.status()).toBe(200);
    await expect(recovered.json()).resolves.toMatchObject({ publicId: published.publicId });
  } finally {
    await clearPublicReadFault(request, ownerA, "get");
  }
});
