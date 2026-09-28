import { afterEach, expect, test, vi } from "vitest";
import { httpClient } from "@/lib/http/client";
import { parsePublicKnowledge, publicKnowledgeApi } from "./public-knowledge-api";

const publicId = "A".repeat(43);
const relatedId = "B".repeat(43);

const markdown = {
  publicId,
  title: "公開知識",
  format: "markdown",
  tags: ["設計", "設計"],
  version: 2,
  updatedAt: "2026-09-20T00:00:00Z",
  publicScope: "unlisted",
  summary: "本文の要約",
  content: { markdown: "# 本文" },
  related: [{ publicId: relatedId, title: "関連記事", format: "markdown", summary: "要約" }],
};

afterEach(() => vi.restoreAllMocks());

test("公開DTOをallow-list検証し、タグと関連記事を上限内へ整える", () => {
  expect(parsePublicKnowledge(markdown)).toMatchObject({
    publicId,
    content: { markdown: "# 本文" },
    tags: ["設計"],
    related: [{ publicId: relatedId }],
  });
  expect(() => parsePublicKnowledge({ ...markdown, owner: "secret" })).not.toThrow();
  expect(() => parsePublicKnowledge({ ...markdown, publicScope: "private" })).toThrow();
  expect(() => parsePublicKnowledge({ ...markdown, publicId: "invalid" })).toThrow();
  expect(() =>
    parsePublicKnowledge({ ...markdown, content: { htmlUrl: "javascript:alert(1)" } }),
  ).toThrow();
});

test("公開HTMLは版固定URLだけを受け入れる", () => {
  const value = {
    ...markdown,
    format: "html",
    content: { htmlUrl: `https://preview.example/public/${publicId}/html?version=2` },
  };

  expect(parsePublicKnowledge(value).content).toEqual(value.content);
  expect(() =>
    parsePublicKnowledge({
      ...value,
      content: { htmlUrl: `https://preview.example/public/${publicId}/html?version=1` },
    }),
  ).toThrow();
});

test("関連記事は不正な項目だけを除外し、本文と正常な項目を維持する", () => {
  const invalid = { publicId: "invalid", title: "非公開候補", format: "markdown", summary: "要約" };

  expect(
    parsePublicKnowledge({ ...markdown, related: [invalid, markdown.related[0]] }),
  ).toMatchObject({
    content: markdown.content,
    related: [{ publicId: relatedId }],
  });
  expect(parsePublicKnowledge({ ...markdown, related: [invalid] })).toMatchObject({
    content: markdown.content,
    related: [],
  });
});

test("公開GETはpublicIdをエンコードし、DTOを検証する", async () => {
  const get = vi.spyOn(httpClient, "get").mockResolvedValue(markdown);
  await publicKnowledgeApi.get(publicId);
  expect(get).toHaveBeenCalledWith(`/public/knowledge/${publicId}`, { signal: undefined });
});
