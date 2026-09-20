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
import { KnowledgeCreatePage } from "../ui/pages/knowledge-create-page";

const id = "11111111-1111-4111-8111-111111111111";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

function setup() {
  const root = createRootRoute({ component: Outlet });

  const create = createRoute({
    getParentRoute: () => root,
    path: "/knowledge/new",
    component: KnowledgeCreatePage,
  });

  const draft = createRoute({
    getParentRoute: () => root,
    path: "/knowledge-drafts/$draftId/edit",
    component: () => <p>編集画面</p>,
  });

  const knowledge = createRoute({
    getParentRoute: () => root,
    path: "/knowledge",
    component: () => <p>一覧</p>,
  });

  const router = createRouter({
    routeTree: root.addChildren([create, draft, knowledge]),
    history: createMemoryHistory({ initialEntries: ["/knowledge/new"] }),
  });

  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });

  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  );
}

test("input選択とdropは同じupload経路を使い、成功時は固定draft routeへ遷移する", async () => {
  const upload = vi
    .spyOn(knowledgeApi, "upload")
    .mockResolvedValue({ draftId: id, editPath: `/knowledge-drafts/${id}/edit` });

  setup();
  await screen.findByText("知識を作成");
  const input = screen.getByLabelText(/ここにファイルをドロップ/);
  fireEvent.change(input, {
    target: { files: [new File(["# memo"], "memo.md", { type: "text/markdown" })] },
  });
  await screen.findByText("編集画面");
  expect(upload).toHaveBeenCalledTimes(1);
  expect(upload.mock.calls[0][0].name).toBe("memo.md");
});

test("処理中は再選択を送信せず、入力境界エラーはAPIを呼ばない", async () => {
  let resolve: ((value: { draftId: string; editPath: string }) => void) | undefined;

  const upload = vi.spyOn(knowledgeApi, "upload").mockImplementation(
    () =>
      new Promise((done) => {
        resolve = done;
      }),
  );

  setup();
  await screen.findByText("知識を作成");
  const input = screen.getByLabelText(/ここにファイルをドロップ/);
  const file = new File(["# memo"], "memo.md");
  fireEvent.change(input, { target: { files: [file] } });
  await screen.findByText("解析中…");
  fireEvent.change(input, { target: { files: [file] } });
  expect(upload).toHaveBeenCalledTimes(1);
  resolve?.({ draftId: id, editPath: `/knowledge-drafts/${id}/edit` });
  await waitFor(() => expect(screen.queryByText("解析中…")).toBeNull());
});

test("空・複数・未対応形式を実入力してもAPIを呼ばない", async () => {
  const upload = vi.spyOn(knowledgeApi, "upload");

  setup();
  await screen.findByText("知識を作成");
  const input = screen.getByLabelText(/ここにファイルをドロップ/);

  fireEvent.change(input, { target: { files: [new File([], "empty.md")] } });
  expect((await screen.findByRole("alert")).textContent).toContain("空のファイル");
  expect(upload).not.toHaveBeenCalled();

  fireEvent.click(screen.getByRole("button", { name: "別のファイルを選ぶ" }));
  fireEvent.change(input, {
    target: {
      files: [new File(["one"], "one.md"), new File(["two"], "two.md")],
    },
  });
  expect((await screen.findByRole("alert")).textContent).toContain("1件だけ");
  expect(upload).not.toHaveBeenCalled();

  fireEvent.click(screen.getByRole("button", { name: "別のファイルを選ぶ" }));
  fireEvent.change(input, { target: { files: [new File(["x"], "note.txt")] } });
  expect((await screen.findByRole("alert")).textContent).toContain(".md");
  expect(upload).not.toHaveBeenCalled();
});
