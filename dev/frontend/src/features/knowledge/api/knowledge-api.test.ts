import { afterEach, expect, test, vi } from "vitest";
import { httpClient } from "@/lib/http/client";
import { knowledgeApi, parseCreatedDraft, parseFolders, parseList } from "./knowledge-api";

const id = "11111111-1111-4111-8111-111111111111";

const folder = { id, name: "仕事", parentId: null, version: 1 };

const item = {
  id,
  title: "仕様書",
  format: "markdown",
  tags: ["設計"],
  folder,
  version: 2,
  updatedAt: "2026-09-20T00:00:00Z",
};

test("一覧とfolderのDTOを検証し、未知の行を受け入れない", () => {
  expect(parseList({ items: [item], total: 1, page: 1, pageSize: 10, hasNext: false })).toEqual({
    items: [item],
    total: 1,
    page: 1,
    pageSize: 10,
    hasNext: false,
  });
  expect(parseFolders({ items: [folder] })).toEqual([folder]);
  expect(() =>
    parseList({
      items: [{ ...item, version: 0 }],
      total: 1,
      page: 1,
      pageSize: 10,
      hasNext: false,
    }),
  ).toThrow();
  expect(() => parseFolders({ items: [{ ...folder, parentId: "bad" }] })).toThrow();
});

test("upload成功DTOは固定のdraft編集pathだけを受け入れる", () => {
  expect(parseCreatedDraft({ draftId: id, editPath: `/knowledge-drafts/${id}/edit` })).toEqual({
    draftId: id,
    editPath: `/knowledge-drafts/${id}/edit`,
  });
  expect(() => parseCreatedDraft({ draftId: id, editPath: "https://evil.example/" })).toThrow();
  expect(() =>
    parseCreatedDraft({ draftId: "bad", editPath: "/knowledge-drafts/bad/edit" }),
  ).toThrow();
});

afterEach(() => vi.restoreAllMocks());

test("一覧の検索条件をAPIの反復queryへ変換する", async () => {
  const get = vi.spyOn(httpClient, "get").mockResolvedValue({
    items: [item],
    total: 1,
    page: 2,
    pageSize: 10,
    hasNext: false,
  });

  await knowledgeApi.list({
    query: "alpha",
    tags: ["one", "two"],
    folderId: id,
    page: 2,
    pageSize: 10,
  });

  expect(get).toHaveBeenCalledWith(
    `/knowledge?query=alpha&tag=one&tag=two&folderId=${id}&page=2&pageSize=10`,
    { signal: undefined },
  );
});
