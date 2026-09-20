import type { APIRequestContext } from "@playwright/test";
import { expect, test } from "../../fixtures/test";
import { authenticatedHeaders } from "../../fixtures/auth/test-session";
import { draftState, failNextUpload } from "../../fixtures/knowledge/test-controls";
import type { FixtureUser } from "../../fixtures/auth/types";

const ownerA: FixtureUser = { key: "owner-a", displayName: "所有者A" };
const ownerB: FixtureUser = { key: "other-b", displayName: "別ユーザーB" };
const maxSourceBytes = 10 * 1024 * 1024;

type UploadPart = { name: string; mimeType: string; body: Buffer };

function multipart(parts: UploadPart[]) {
  const boundary = "e2e-knowledge-upload-boundary";

  const body = Buffer.concat(
    parts.flatMap((part) => [
      Buffer.from(`--${boundary}\r\n`),
      Buffer.from(
        `Content-Disposition: form-data; name="file"; filename="${part.name}"\r\nContent-Type: ${part.mimeType}\r\n\r\n`,
      ),
      part.body,
      Buffer.from("\r\n"),
    ]),
  );

  return {
    data: Buffer.concat([body, Buffer.from(`--${boundary}--\r\n`)]),
    headers: {
      ...authenticatedHeaders(ownerA),
      "Content-Type": `multipart/form-data; boundary=${boundary}`,
    },
  };
}

async function upload(request: APIRequestContext, part: UploadPart) {
  return request.post("/api/v1/knowledge-drafts", multipart([part]));
}

function responseBody(response: { json: () => Promise<unknown> }) {
  return response.json() as Promise<Record<string, unknown>>;
}

// Issue #15 X02：実multipart入口のステータス・公開DTO・実DB/MinIOへのdraft作成を確認する。
test("Markdownをmultipartで送ると、編集URL付きの下書きが作成される", async ({ request }) => {
  const response = await upload(request, {
    name: "contract.md",
    mimeType: "text/markdown",
    body: Buffer.from("# 契約Markdown\n\n本文", "utf8"),
  });

  expect(response.status()).toBe(201);
  const body = await responseBody(response);
  if (typeof body.draftId !== "string") throw new Error("E2E upload did not return a draft id");
  expect(body.editPath).toBe(`/knowledge-drafts/${body.draftId}/edit`);

  const draft = await request.get(`/api/v1/knowledge-drafts/${body.draftId}`, {
    headers: authenticatedHeaders(ownerA),
  });

  expect(draft.status()).toBe(200);
  await expect(draft.json()).resolves.toMatchObject({
    draftId: body.draftId,
    status: "pending",
    format: "markdown",
    source: "# 契約Markdown\n\n本文",
  });
});

// Issue #15 X02：HTMLも同じmultipart契約へ接続し、ownerを跨ぐdraft非開示を確認する。
test("HTMLをmultipartで送ると、別所有者からは同じ下書きを取得できない", async ({ request }) => {
  const response = await upload(request, {
    name: "contract.html",
    mimeType: "text/html",
    body: Buffer.from("<h1>契約HTML</h1>", "utf8"),
  });

  expect(response.status()).toBe(201);
  const body = await responseBody(response);
  const draftId = body.draftId as string;

  const other = await request.get(`/api/v1/knowledge-drafts/${draftId}`, {
    headers: authenticatedHeaders(ownerB),
  });

  const missing = await request.get(
    "/api/v1/knowledge-drafts/00000000-0000-4000-8000-000000000000",
    {
      headers: authenticatedHeaders(ownerB),
    },
  );

  expect(other.status()).toBe(404);
  expect(missing.status()).toBe(404);
  await expect(other.json()).resolves.toEqual(await missing.json());
});

// Issue #15 X02：入力境界はAPI契約で確認し、ブラウザspecへ大量データを重複させない。
test("空・複数・未対応形式・壊れたfront matterは下書きを作成せず、再送可能なエラーを返す", async ({
  request,
}) => {
  const cases: Array<{
    name: string;
    mimeType: string;
    body: Buffer;
    status: number;
    reason?: string;
  }> = [
    {
      name: "empty.md",
      mimeType: "text/markdown",
      body: Buffer.alloc(0),
      status: 422,
      reason: "empty_source",
    },
    { name: "unsupported.txt", mimeType: "text/plain", body: Buffer.from("本文"), status: 400 },
    {
      name: "broken.md",
      mimeType: "text/markdown",
      body: Buffer.from("---\ntitle: [壊れた\n---\n\n本文"),
      status: 422,
      reason: "front_matter_syntax",
    },
  ];

  for (const input of cases) {
    const before = await draftState(request, ownerA);
    const response = await upload(request, input);
    expect(response.status()).toBe(input.status);
    const body = await responseBody(response);
    expect(body.error).toMatchObject({
      code: input.status === 422 ? "validation_failed" : "bad_request",
    });
    if (input.reason) expect(body.error).toMatchObject({ details: [{ reason: input.reason }] });
    await expect(draftState(request, ownerA)).resolves.toEqual(before);
  }

  const beforeMultiple = await draftState(request, ownerA);

  const multiple = await request.post(
    "/api/v1/knowledge-drafts",
    multipart([
      { name: "one.md", mimeType: "text/markdown", body: Buffer.from("one") },
      { name: "two.md", mimeType: "text/markdown", body: Buffer.from("two") },
    ]),
  );

  expect(multiple.status()).toBe(400);
  await expect(draftState(request, ownerA)).resolves.toEqual(beforeMultiple);
});

test("10 MiBちょうどのmultipartは作成でき、1 byte超過は413になる", async ({ request }) => {
  const boundary = await upload(request, {
    name: "boundary.md",
    mimeType: "text/markdown",
    body: Buffer.alloc(maxSourceBytes, "x"),
  });

  expect(boundary.status()).toBe(201);
  const afterBoundary = await draftState(request, ownerA);

  const beforeTooLarge = await draftState(request, ownerA);

  const tooLarge = await upload(request, {
    name: "too-large.md",
    mimeType: "text/markdown",
    body: Buffer.alloc(maxSourceBytes + 1, "x"),
  });

  expect(tooLarge.status()).toBe(413);
  await expect(tooLarge.json()).resolves.toMatchObject({ error: { code: "payload_too_large" } });
  await expect(draftState(request, ownerA)).resolves.toEqual(beforeTooLarge);
  await expect(draftState(request, ownerA)).resolves.toEqual(afterBoundary);
});

test("DBまたはS3のupload障害は503で下書きを増やさず、復旧後に再試行できる", async ({ request }) => {
  const source = "# 障害復旧\n\n再試行で保存される本文";

  for (const kind of ["database", "object"] as const) {
    const before = await draftState(request, ownerA);
    await failNextUpload(request, ownerA, kind);

    const failed = await upload(request, {
      name: `${kind}-failure.md`,
      mimeType: "text/markdown",
      body: Buffer.from(source, "utf8"),
    });

    expect(failed.status()).toBe(503);
    const afterFailure = await draftState(request, ownerA);
    expect(afterFailure).toEqual(before);

    const retry = await upload(request, {
      name: `${kind}-recovered.md`,
      mimeType: "text/markdown",
      body: Buffer.from(source, "utf8"),
    });

    expect(retry.status()).toBe(201);
    const afterRetry = await draftState(request, ownerA);
    expect(afterRetry.pending).toBe(before.pending + 1);
    expect(afterRetry.total).toBe(before.total + 1);
  }
});
