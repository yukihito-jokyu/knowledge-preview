import { afterEach, expect, test, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  Outlet,
  RouterProvider,
} from "@tanstack/react-router";
import { publicKnowledgeQueryKeys, type PublicKnowledge } from "../../model/public-knowledge";
import { PublicKnowledgePage } from "./public-knowledge-page";

const publicId = "A".repeat(43);
let calls = 0;

const knowledge: PublicKnowledge = {
  publicId,
  title: "公開知識",
  format: "markdown",
  tags: ["設計"],
  version: 2,
  updatedAt: "2026-09-20T00:00:00Z",
  publicScope: "unlisted",
  summary: "本文の要約",
  content: { markdown: "# 本文\n\n> 要点" },
  related: [],
};

let loadPublicKnowledge: () => Promise<PublicKnowledge> = async () => knowledge;

vi.mock("../../api/public-knowledge-api", () => ({
  publicKnowledgeApi: {
    get: () => {
      calls += 1;
      return loadPublicKnowledge();
    },
  },
}));

afterEach(() => {
  cleanup();
  calls = 0;
  loadPublicKnowledge = async () => knowledge;
  vi.restoreAllMocks();
});

function renderPage(
  id = publicId,
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } }),
) {
  const root = createRootRoute({ component: Outlet });

  const route = createRoute({
    getParentRoute: () => root,
    path: "/public/knowledge/$publicId",
    component: () => <PublicKnowledgePage publicId={id} />,
  });

  const router = createRouter({
    routeTree: root.addChildren([route]),
    history: createMemoryHistory({ initialEntries: [`/public/knowledge/${id}`] }),
  });

  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  );
  return client;
}

test("不正なpublicIdはAPIを呼ばず閲覧不可を表示する", async () => {
  renderPage("invalid");
  expect(await screen.findByRole("heading", { name: "この知識は閲覧できません" })).toBeTruthy();
  expect(calls).toBe(0);
});

test("公開Markdownを表示し、読み込み失敗時は本文を残さない", async () => {
  renderPage();
  expect(await screen.findByRole("heading", { name: "公開知識" })).toBeTruthy();
  expect(screen.getByText("Knowledge Preview")).toBeTruthy();

  cleanup();
  loadPublicKnowledge = async () => {
    throw new Error("unavailable");
  };
  renderPage();
  expect(await screen.findByRole("heading", { name: "読み込みに失敗しました" })).toBeTruthy();
  expect(screen.queryByText("公開知識")).toBeNull();
});

test("通信失敗は再試行で本文を表示する", async () => {
  let attempts = 0;
  loadPublicKnowledge = async () => {
    attempts += 1;
    if (attempts === 1) throw new Error("unavailable");
    return knowledge;
  };
  renderPage();
  await screen.findByRole("alert");
  fireEvent.click(screen.getByRole("button", { name: "再試行" }));
  expect(await screen.findByRole("heading", { name: "公開知識" })).toBeTruthy();
  expect(attempts).toBe(2);
});

test("再取得中はキャッシュ済みの本文を隠し、成功した新版だけを表示する", async () => {
  let resolve: (value: PublicKnowledge) => void = () => {};
  loadPublicKnowledge = () => new Promise((fulfill) => (resolve = fulfill));
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  client.setQueryData(publicKnowledgeQueryKeys.detail(publicId), knowledge);

  renderPage(publicId, client);
  expect((await screen.findByRole("status")).textContent).toContain("公開知識を読み込んでいます");
  expect(screen.queryByRole("heading", { name: "公開知識" })).toBeNull();

  resolve({ ...knowledge, title: "公開知識・新版" });
  expect(await screen.findByRole("heading", { name: "公開知識・新版" })).toBeTruthy();
});

test("再取得に失敗してもキャッシュ済みの本文を表示しない", async () => {
  let reject: (error: Error) => void = () => {};
  loadPublicKnowledge = () => new Promise((_, fail) => (reject = fail));
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  client.setQueryData(publicKnowledgeQueryKeys.detail(publicId), knowledge);

  renderPage(publicId, client);
  expect((await screen.findByRole("status")).textContent).toContain("公開知識を読み込んでいます");
  expect(screen.queryByRole("heading", { name: "公開知識" })).toBeNull();

  reject(new Error("公開知識を取得できません"));
  expect(await screen.findByRole("heading", { name: "読み込みに失敗しました" })).toBeTruthy();
  expect(screen.queryByRole("heading", { name: "公開知識" })).toBeNull();
});

test("Markdownの引用は本文のblockquote描画で継続行・複数引用・引用内codeを保持する", async () => {
  loadPublicKnowledge = async () => ({
    ...knowledge,
    content: {
      markdown: "# 本文\n\n> 最初の要点\n> 継続行\n> `引用内コード`\n\n> 二つ目の要点",
    },
  });

  renderPage();
  expect(await screen.findByRole("heading", { name: "公開知識" })).toBeTruthy();
  expect(screen.getByText(/継続行/)).toBeTruthy();
  expect(screen.getByText("引用内コード")).toBeTruthy();
  expect(screen.getByText("二つ目の要点")).toBeTruthy();
  expect(document.querySelectorAll("blockquote")).toHaveLength(2);
});
