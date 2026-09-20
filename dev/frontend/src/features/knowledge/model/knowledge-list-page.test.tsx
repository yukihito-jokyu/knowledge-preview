import { afterEach, expect, test, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  Outlet,
  RouterProvider,
} from "@tanstack/react-router";
import { knowledgeApi } from "../api/knowledge-api";
import { KnowledgeListPage } from "../ui/pages/knowledge-list-page";
import type { Folder, KnowledgeListItem } from "./knowledge-types";
import { ApiError } from "@/lib/http/api-error";

const folderId = "11111111-1111-4111-8111-111111111111";
const itemId = "22222222-2222-4222-8222-222222222222";
const folder: Folder = { id: folderId, name: "仕事", parentId: null, version: 1 };

const item: KnowledgeListItem = {
  id: itemId,
  title: "知識",
  format: "markdown",
  updatedAt: "2026-09-20T00:00:00Z",
  tags: ["one", "two"],
  folder: null,
  version: 1,
};

type Search = {
  q?: string;
  tag?: string[];
  folderId?: string;
  page?: number;
  pageSize?: number;
};

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

function setup(search: Search = {}) {
  const root = createRootRoute({ component: Outlet });

  const knowledgeRoute = createRoute({
    getParentRoute: () => root,
    path: "/knowledge",
    validateSearch: (value: Record<string, unknown>): Search => ({
      q: typeof value.q === "string" ? value.q : undefined,
      tag: Array.isArray(value.tag)
        ? value.tag.filter((tag): tag is string => typeof tag === "string")
        : typeof value.tag === "string"
          ? [value.tag]
          : undefined,
      folderId: typeof value.folderId === "string" ? value.folderId : undefined,
      page: typeof value.page === "number" ? value.page : 1,
      pageSize: typeof value.pageSize === "number" ? value.pageSize : 10,
    }),
    component: () => <KnowledgeListPage search={knowledgeRoute.useSearch()} />,
  });

  const create = createRoute({
    getParentRoute: () => root,
    path: "/knowledge/new",
    component: () => <p>作成</p>,
  });

  const edit = createRoute({
    getParentRoute: () => root,
    path: "/knowledge/$knowledgeId/edit",
    component: () => <p>編集</p>,
  });

  const router = createRouter({
    routeTree: root.addChildren([knowledgeRoute, create, edit]),
    history: createMemoryHistory({ initialEntries: [`/knowledge${searchParams(search)}`] }),
  });

  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });

  vi.spyOn(knowledgeApi, "folders").mockResolvedValue([folder]);
  vi.spyOn(knowledgeApi, "list").mockImplementation(async (params) => ({
    items: [item],
    total: 11,
    page: params.page,
    pageSize: params.pageSize,
    hasNext: params.page < 2,
  }));

  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  );
  return router;
}

function searchParams(search: Search): string {
  const params = new URLSearchParams();
  if (search.q) params.set("q", search.q);
  for (const tag of search.tag ?? []) params.append("tag", tag);
  if (search.folderId) params.set("folderId", search.folderId);
  if (search.page !== undefined) params.set("page", String(search.page));
  if (search.pageSize !== undefined) params.set("pageSize", String(search.pageSize));
  const value = params.toString();
  return value ? `?${value}` : "";
}

test("folder保存はform submitを含めてpending中に一度だけ送信し、取消でopenerへ戻る", async () => {
  let resolve: ((value: Folder) => void) | undefined;

  const createFolder = vi.spyOn(knowledgeApi, "createFolder").mockImplementation(
    () =>
      new Promise((done) => {
        resolve = done;
      }),
  );

  const router = setup();
  await screen.findByText("知識一覧");

  const opener = screen.getByRole("button", { name: "追加" });
  fireEvent.click(opener);
  fireEvent.change(screen.getByRole("textbox", { name: "フォルダ名" }), {
    target: { value: "新しいフォルダ" },
  });
  const form = screen.getByRole("dialog").querySelector("form");
  expect(form).not.toBeNull();
  fireEvent.submit(form as HTMLFormElement);
  await waitFor(() => expect(createFolder).toHaveBeenCalledTimes(1));
  fireEvent.submit(form as HTMLFormElement);
  expect(createFolder).toHaveBeenCalledTimes(1);

  resolve?.({ ...folder, name: "新しいフォルダ" });
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());

  fireEvent.click(opener);
  fireEvent.click(screen.getByRole("button", { name: "キャンセル" }));
  await waitFor(() => expect(document.activeElement).toBe(opener));
  expect(router.state.location.pathname).toBe("/knowledge");
});

test("folder作成の同名409は再読み込みではなく別名を案内する", async () => {
  const createFolder = vi
    .spyOn(knowledgeApi, "createFolder")
    .mockRejectedValue(new ApiError("conflict", 409));

  setup();
  await screen.findByText("知識一覧");

  fireEvent.click(screen.getByRole("button", { name: "追加" }));
  fireEvent.change(screen.getByRole("textbox", { name: "フォルダ名" }), {
    target: { value: "仕事" },
  });
  fireEvent.submit(screen.getByRole("dialog").querySelector("form") as HTMLFormElement);

  await waitFor(() => expect(createFolder).toHaveBeenCalledWith({ name: "仕事", parentId: null }));
  const alert = await screen.findByRole("alert");
  expect(alert.textContent).toContain("同名のフォルダがあります");
  expect(alert.textContent).not.toContain("再読み込み");
});

test("folder変更の409は同名重複と最新状態競合の両方を案内する", async () => {
  const updateFolder = vi
    .spyOn(knowledgeApi, "updateFolder")
    .mockRejectedValue(new ApiError("conflict", 409));

  setup();
  await screen.findByText("知識一覧");
  await screen.findByRole("button", { name: "仕事" });

  fireEvent.click(screen.getByRole("button", { name: "仕事を変更" }));
  fireEvent.change(screen.getByRole("textbox", { name: "フォルダ名" }), {
    target: { value: "仕事" },
  });
  fireEvent.submit(screen.getByRole("dialog").querySelector("form") as HTMLFormElement);

  await waitFor(() =>
    expect(updateFolder).toHaveBeenCalledWith("11111111-1111-4111-8111-111111111111", 1, {
      name: "仕事",
      parentId: null,
    }),
  );
  const alert = await screen.findByRole("alert");
  expect(alert.textContent).toContain("同名のフォルダがあるか");
  expect(alert.textContent).toContain("別の操作で更新されています");
});

test("選択中folder削除成功時は親folderとpage1へ移動する", async () => {
  let deleted = false;
  vi.spyOn(knowledgeApi, "deleteFolder").mockImplementation(async () => {
    deleted = true;
  });
  const router = setup({ folderId, page: 2, pageSize: 10 });
  vi.mocked(knowledgeApi.folders).mockImplementation(async () => (deleted ? [] : [folder]));
  await screen.findByText("知識一覧");
  await waitFor(() => expect(router.state.location.search).toMatchObject({ folderId, page: 2 }));

  fireEvent.click(screen.getByRole("button", { name: "仕事を削除" }));
  fireEvent.click(screen.getByRole("button", { name: "削除" }));
  await waitFor(() => expect(router.state.location.search).toMatchObject({ page: 1 }));
  expect(router.state.location.search.folderId).toBeUndefined();
});

test("tag選択はquery・folder・pageSizeを保ち、複数追加と解除をURLへ反映する", async () => {
  const router = setup({ q: "term", folderId, page: 2, pageSize: 25 });
  await screen.findByText("知識一覧");
  const one = await screen.findByRole("link", { name: "#one" });
  expect(one.getAttribute("href")).toContain("tag=%5B%22one%22%5D");
  expect(one.getAttribute("href")).toContain("page=1");
  expect(one.getAttribute("href")).toContain("pageSize=25");
  await router.navigate({
    to: "/knowledge",
    search: { q: "term", folderId, page: 1, pageSize: 25, tag: ["one"] },
  });
  expect(router.state.location.search.tag).toEqual(["one"]);

  const two = await screen.findByRole("link", { name: "#two" });
  expect(two.getAttribute("href")).toContain("tag=%5B%22one%22%2C%22two%22%5D");
  await router.navigate({
    to: "/knowledge",
    search: { q: "term", folderId, page: 1, pageSize: 25, tag: ["one", "two"] },
  });
  expect(router.state.location.search.tag).toEqual(["one", "two"]);

  const oneAgain = await screen.findByRole("link", { name: "#one" });
  expect(oneAgain.getAttribute("href")).toContain("tag=%5B%22two%22%5D");
});

test("pageSize選択はpage1へ戻し、totalを超えるpageは最終pageへ補正する", async () => {
  const router = setup({ page: 3, pageSize: 10 });
  await screen.findByText("知識一覧");
  await waitFor(() => expect(router.state.location.search.page).toBe(2));

  fireEvent.change(screen.getByRole("combobox", { name: "表示件数" }), {
    target: { value: "25" },
  });
  await waitFor(() =>
    expect(router.state.location.search).toMatchObject({ page: 1, pageSize: 25 }),
  );
});

test("201 Unicode code pointsの検索語はAPIを呼ばず入力欄の近くへ表示する", async () => {
  const router = setup({ q: "😀".repeat(201) });
  const list = vi.mocked(knowledgeApi.list);
  await screen.findByText("知識一覧");

  expect(screen.getByRole("alert").textContent).toContain("200文字");
  expect(list).not.toHaveBeenCalled();
  expect(router.state.location.search.q).toBe("😀".repeat(201));
});
