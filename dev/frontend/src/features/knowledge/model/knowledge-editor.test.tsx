import { afterEach, expect, test, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import {
  QueryClient,
  QueryClientProvider,
  focusManager,
  onlineManager,
} from "@tanstack/react-query";
import {
  createRootRoute,
  createRoute,
  createRouter,
  createMemoryHistory,
  RouterProvider,
  Outlet,
} from "@tanstack/react-router";
import { knowledgeApi, parseDetail, parseRecent } from "../api/knowledge-api";
import { KnowledgeEditPage } from "../ui/pages/knowledge-edit-page";
import { ApiError } from "@/lib/http/api-error";
import {
  knowledgeError,
  markdownBody,
  parseViewMode,
  safePreviewUrl,
  validateSource,
} from "./knowledge-editor";
import type { KnowledgeDetail } from "./knowledge-types";

const id = "11111111-1111-4111-8111-111111111111";

const detail: KnowledgeDetail = {
  id,
  title: "Docker",
  format: "markdown",
  source: "# Docker\n旧本文",
  tags: ["Docker"],
  learningStatus: "learning",
  folder: null,
  version: 1,
  updatedAt: "2026-09-12T10:00:00Z",
  visibility: "private",
  publicId: null,
  publicUrl: null,
};

afterEach(() => {
  cleanup();
  focusManager.setFocused(undefined);
  onlineManager.setOnline(true);
  vi.restoreAllMocks();
});

function setup(value = detail, draft = false) {
  vi.spyOn(knowledgeApi, "detail").mockResolvedValue(value);
  vi.spyOn(knowledgeApi, "draft").mockResolvedValue({ ...value, draftId: id, status: "pending" });
  vi.spyOn(knowledgeApi, "recent").mockResolvedValue([]);

  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });

  const root = createRootRoute({ component: Outlet });

  const route = createRoute({
    getParentRoute: () => root,
    path: "/",
    component: () => <KnowledgeEditPage id={id} draft={draft} mode="split" />,
  });

  const savedRoute = createRoute({
    getParentRoute: () => root,
    path: "/knowledge/$knowledgeId/edit",
    component: () => <KnowledgeEditPage id={id} mode="split" />,
  });

  const router = createRouter({
    routeTree: root.addChildren([route, savedRoute]),
    history: createMemoryHistory({ initialEntries: ["/"] }),
  });

  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  );
}

test("不正なrecent行だけ除外し未知formatのdetailを拒否する", () => {
  expect(parseRecent({ items: [detail, { id: "bad" }, detail] })).toEqual([detail]);
  expect(() => parseDetail({ ...detail, format: "svg" })).toThrow();
});

test("UTF-8の容量・位置エラー・mode・HTML隔離境界", () => {
  expect(validateSource("a".repeat(10 * 1024 * 1024))).toBeUndefined();
  expect(validateSource("あ".repeat(4 * 1024 * 1024))).toContain("10 MiB");
  expect(validateSource(" \n ")).toBeDefined();
  expect(markdownBody("---\ntitle: hi\n---\n# 本文")).toBe("# 本文");
  expect(parseViewMode("unknown")).toBe("preview");
  expect(
    knowledgeError(new ApiError("secret", 422, "validation_failed", [{ line: 3, column: 8 }])),
  ).toContain("3行8列");
  expect(safePreviewUrl("https://app.test:9999/preview", "https://app.test/")).toBeUndefined();
  expect(safePreviewUrl("javascript:alert(1)", "https://app.test/")).toBeUndefined();
  expect(safePreviewUrl("https://preview.test/private", "https://app.test/")).toBe(
    "https://preview.test/private",
  );
});

test("409でも入力を保持しモード切替では取得しない", async () => {
  const save = vi.spyOn(knowledgeApi, "save").mockRejectedValue(new ApiError("conflict", 409));
  setup();
  const input = await screen.findByLabelText(/原文/);
  fireEvent.change(input, { target: { value: "変更した本文" } });
  fireEvent.click(screen.getByRole("button", { name: "保存" }));
  await screen.findByText(/別の操作で更新/);
  fireEvent.click(screen.getByRole("button", { name: "プレビュー" }));
  fireEvent.click(screen.getByRole("button", { name: "エディター" }));
  expect(screen.getByLabelText<HTMLTextAreaElement>(/原文/).value).toBe("変更した本文");
  expect(save).toHaveBeenCalledWith(id, { version: 1, source: "変更した本文" });
  expect(knowledgeApi.detail).toHaveBeenCalledTimes(1);
});

test("保存中の追加入力を成功応答で消さない", async () => {
  let complete: ((value: KnowledgeDetail) => void) | undefined;
  vi.spyOn(knowledgeApi, "save").mockImplementation(
    () =>
      new Promise((resolve) => {
        complete = resolve;
      }),
  );
  setup();
  const input = await screen.findByLabelText(/原文/);
  fireEvent.change(input, { target: { value: "送信本文" } });
  fireEvent.click(screen.getByRole("button", { name: "保存" }));
  await waitFor(() => expect(complete).toBeDefined());
  fireEvent.change(input, { target: { value: "送信後の編集" } });
  complete?.({ ...detail, source: "送信本文", version: 2 });
  await screen.findByText("保存しました。");
  expect(screen.getByLabelText<HTMLTextAreaElement>(/原文/).value).toBe("送信後の編集");
  expect(screen.getByText("未保存の変更があります")).toBeDefined();
});

test("HTMLは保存成功時だけticketを再発行し失敗時は旧表示を維持する", async () => {
  const preview = vi
    .spyOn(knowledgeApi, "preview")
    .mockResolvedValue("https://preview.test/private");

  const save = vi
    .spyOn(knowledgeApi, "save")
    .mockRejectedValueOnce(new ApiError("unavailable", 503))
    .mockResolvedValue({
      ...detail,
      format: "html",
      source: "<p>新本文</p>",
      version: 2,
      warnings: [{ code: "html_sanitized" }],
    });

  setup({ ...detail, format: "html", source: "<p>旧本文</p>" });
  await screen.findByTitle("保存済みHTMLプレビュー");
  fireEvent.change(screen.getByLabelText(/原文/), { target: { value: "<p>新本文</p>" } });
  expect(preview).toHaveBeenCalledTimes(1);
  fireEvent.click(screen.getByRole("button", { name: "保存" }));
  await screen.findByText(/保存できませんでした/);
  expect(preview).toHaveBeenCalledTimes(1);
  fireEvent.click(screen.getByRole("button", { name: "保存" }));
  await waitFor(() => expect(preview).toHaveBeenCalledWith(id, 2));
  expect(save).toHaveBeenCalledTimes(2);
  expect(screen.getAllByText("安全のためHTMLの一部を除去して表示しています。")).toHaveLength(1);
  const iframe = await screen.findByTitle("保存済みHTMLプレビュー");
  expect(iframe.getAttribute("sandbox")).toBe("");
  expect(iframe.getAttribute("referrerpolicy")).toBe("no-referrer");
  expect(iframe.hasAttribute("allow")).toBe(false);
  expect(iframe.hasAttribute("srcdoc")).toBe(false);
});

test("draft保存失敗後の確認で確定済みIDへ回復でき入力を保持する", async () => {
  vi.spyOn(knowledgeApi, "commit").mockRejectedValue(new ApiError("lost response", 0));
  setup(detail, true);
  const input = await screen.findByLabelText(/原文/);
  fireEvent.change(input, { target: { value: "確定本文" } });
  fireEvent.click(screen.getByRole("button", { name: "正式保存" }));
  await screen.findByRole("button", { name: "正式保存の結果を確認" });
  vi.mocked(knowledgeApi.draft).mockResolvedValue({
    draftId: id,
    status: "committed",
    knowledgeId: id,
    editPath: `/knowledge/${id}/edit`,
  });
  fireEvent.click(screen.getByRole("button", { name: "正式保存の結果を確認" }));
  await screen.findByText("保存済みの知識を開く（未保存入力は破棄）");
  expect(screen.getByLabelText<HTMLTextAreaElement>(/原文/).value).toBe("確定本文");
});

for (const trigger of ["focus", "reconnect"] as const) {
  function refetch() {
    if (trigger === "focus") {
      focusManager.setFocused(false);
      focusManager.setFocused(true);
    } else {
      onlineManager.setOnline(false);
      onlineManager.setOnline(true);
    }
  }

  test(`${trigger}再取得でdraft確定が判明しても追加入力と破棄確認を保持する`, async () => {
    vi.spyOn(knowledgeApi, "commit").mockRejectedValue(new ApiError("lost response", 0));
    setup(detail, true);
    const input = await screen.findByLabelText(/原文/);
    fireEvent.change(input, { target: { value: "確定本文" } });
    fireEvent.click(screen.getByRole("button", { name: "正式保存" }));
    await screen.findByRole("button", { name: "正式保存の結果を確認" });
    fireEvent.change(input, { target: { value: "失敗後の追加入力" } });
    vi.mocked(knowledgeApi.draft).mockResolvedValue({
      draftId: id,
      status: "committed",
      knowledgeId: id,
      editPath: `/knowledge/${id}/edit`,
    });
    refetch();
    const recovery = await screen.findByText("保存済みの知識を開く（未保存入力は破棄）");
    expect(knowledgeApi.draft).toHaveBeenCalledTimes(2);
    expect(screen.getByLabelText<HTMLTextAreaElement>(/原文/).value).toBe("失敗後の追加入力");
    fireEvent.click(recovery);
    await screen.findByText("未保存の変更を破棄しますか？");
    fireEvent.click(screen.getByRole("button", { name: "編集を続ける" }));
    expect(screen.getByLabelText<HTMLTextAreaElement>(/原文/).value).toBe("失敗後の追加入力");
  });

  test(`${trigger}再取得でもHTMLの本文・保存版・メタデータを一緒に保持する`, async () => {
    const preview = vi
      .spyOn(knowledgeApi, "preview")
      .mockResolvedValue("https://preview.test/private");

    const save = vi.spyOn(knowledgeApi, "save").mockRejectedValue(new ApiError("conflict", 409));
    setup({ ...detail, format: "html", source: "<p>旧本文</p>" });
    await screen.findByTitle("保存済みHTMLプレビュー");
    vi.mocked(knowledgeApi.detail).mockResolvedValue({
      ...detail,
      format: "html",
      title: "別タブの更新",
      tags: ["新タグ"],
      source: "<p>別タブの本文</p>",
      version: 2,
      visibility: "unlisted",
      publicId: id,
    });
    refetch();
    await waitFor(() => expect(knowledgeApi.detail).toHaveBeenCalledTimes(2));
    expect(screen.getByRole("heading", { name: "Docker" })).toBeDefined();
    expect(screen.queryByText("# 新タグ")).toBeNull();
    expect(screen.getByLabelText<HTMLTextAreaElement>(/原文/).value).toBe("<p>旧本文</p>");
    fireEvent.change(screen.getByLabelText(/原文/), { target: { value: "<p>未保存入力</p>" } });
    refetch();
    await waitFor(() => expect(knowledgeApi.detail).toHaveBeenCalledTimes(3));
    expect(screen.getByRole("heading", { name: "Docker" })).toBeDefined();
    expect(preview).toHaveBeenCalledTimes(1);
    expect(screen.getByLabelText<HTMLTextAreaElement>(/原文/).value).toBe("<p>未保存入力</p>");
    fireEvent.click(screen.getByRole("button", { name: "保存" }));
    await screen.findByText(/別の操作で更新/);
    expect(save).toHaveBeenCalledWith(id, { version: 1, source: "<p>未保存入力</p>" });
  });
}

for (const entry of ["initial", "commit", "recovery"] as const) {
  test(`${entry}で保存済みHTMLへ入ると除去通知を表示する`, async () => {
    const html: KnowledgeDetail = {
      ...detail,
      format: "html",
      source: "<p>本文</p><script>alert(1)</script>",
      warnings: [{ code: "html_sanitized" }],
    };

    vi.spyOn(knowledgeApi, "preview").mockResolvedValue("https://preview.test/private");
    const commit = vi.spyOn(knowledgeApi, "commit");
    if (entry === "recovery") commit.mockRejectedValue(new ApiError("lost response", 0));
    else commit.mockResolvedValue({ knowledgeId: id, editPath: `/knowledge/${id}/edit` });
    setup(html, entry !== "initial");
    await screen.findByLabelText(/原文/);
    if (entry !== "initial") {
      fireEvent.click(screen.getByRole("button", { name: "正式保存" }));
      if (entry === "recovery") {
        await screen.findByRole("button", { name: "正式保存の結果を確認" });
        vi.mocked(knowledgeApi.draft).mockResolvedValue({
          draftId: id,
          status: "committed",
          knowledgeId: id,
          editPath: `/knowledge/${id}/edit`,
        });
        fireEvent.click(screen.getByRole("button", { name: "正式保存の結果を確認" }));
        fireEvent.click(await screen.findByText("保存済みの知識を開く（未保存入力は破棄）"));
      }
    }
    await screen.findByTitle("保存済みHTMLプレビュー");
    expect(screen.getAllByText("安全のためHTMLの一部を除去して表示しています。")).toHaveLength(1);
    expect(knowledgeApi.detail).toHaveBeenCalled();

    const save = vi.spyOn(knowledgeApi, "save").mockResolvedValue({
      ...html,
      source: "<p>安全な本文</p>",
      version: 2,
      warnings: [],
    });

    fireEvent.change(screen.getByLabelText(/原文/), { target: { value: "<p>安全な本文</p>" } });
    fireEvent.click(screen.getByRole("button", { name: "保存" }));
    await screen.findByText("保存しました。");
    expect(save).toHaveBeenCalled();
    expect(screen.queryByText("安全のためHTMLの一部を除去して表示しています。")).toBeNull();
  });
}
