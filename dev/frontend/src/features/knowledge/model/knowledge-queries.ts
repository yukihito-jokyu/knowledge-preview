import { queryOptions, useMutation, useQueryClient } from "@tanstack/react-query";
import { knowledgeApi } from "../api/knowledge-api";
import { knowledgeQueryKeys } from "./knowledge-query-keys";
import { ApiError } from "@/lib/http/api-error";
import type { FolderInput, KnowledgeListParams, SaveInput, Visibility } from "./knowledge-types";
export {
  knowledgeError,
  markdownBody,
  parseViewMode,
  safePreviewUrl,
  validateSource,
} from "./knowledge-editor";
export type { ViewMode } from "./knowledge-editor";

const maxUploadBytes = 10 * 1024 * 1024;
const maxSearchQueryCodePoints = 200;

export type FolderOperation = "create" | "update" | "delete" | "move";

export function validateSearchQuery(value: string): string | undefined {
  return Array.from(value.trim()).length > maxSearchQueryCodePoints
    ? "検索語は200文字以内で入力してください。"
    : undefined;
}

export function folderOperationError(error: unknown, operation: FolderOperation): string {
  const name =
    operation === "create"
      ? "作成"
      : operation === "update"
        ? "変更"
        : operation === "delete"
          ? "削除"
          : "移動";

  if (!(error instanceof ApiError)) return `フォルダの${name}に失敗しました。再試行してください。`;
  if (error.status === 401) return "ログインの有効期限が切れました。再ログインしてください。";
  if (error.status === 404)
    return "フォルダが見つかりません。再読み込みしてから再試行してください。";
  if (error.status === 409) {
    if (operation === "delete")
      return "フォルダを削除できません。中身があるか、別の操作で更新されています。";
    if (operation === "create")
      return "同じ親フォルダに同名のフォルダがあります。別の名前を入力してください。";
    if (operation === "move")
      return "フォルダが別の操作で更新されています。再読み込みしてから再試行してください。";
    return "同名のフォルダがあるか、別の操作で更新されています。別の名前を試すか、再読み込みしてから再試行してください。";
  }
  if (error.status === 422)
    return operation === "delete"
      ? "フォルダを削除できません。フォルダの状態を確認してください。"
      : "フォルダ名または親フォルダを確認してください。";
  return `フォルダの${name}に失敗しました。再試行してください。`;
}

export function validateUploadFiles(files: FileList | File[]): string | undefined {
  if (files.length !== 1) return "ファイルは1件だけ選択してください。";
  const file = files[0];
  const extension = file.name.toLowerCase().slice(file.name.lastIndexOf("."));
  if (extension !== ".md" && extension !== ".html")
    return "Markdown（.md）またはHTML（.html）を選択してください。";
  if (file.size === 0) return "空のファイルは選択できません。";
  if (file.size > maxUploadBytes) return "ファイルは10 MiB以内にしてください。";
}

export function uploadError(error: unknown): string {
  if (!(error instanceof ApiError)) return "解析できませんでした。別のファイルを選んでください。";
  if (error.status === 413) return "ファイルは10 MiB以内にしてください。";
  if (error.status === 422) {
    const detail = error.details.find((item) => item.reason);
    const position = detail?.line ? ` ${detail.line}行${detail.column ?? 1}列` : "";
    return `ファイルの内容を確認してください。${position}`;
  }
  if (error.status === 400) return "ファイル形式または送信内容を確認してください。";
  return "解析できませんでした。別のファイルを選んでください。";
}

export const detailOptions = (id: string) =>
  queryOptions({
    queryKey: knowledgeQueryKeys.detail(id),
    queryFn: ({ signal }) => knowledgeApi.detail(id, signal),
    retry: false,
  });

export const draftOptions = (id: string) =>
  queryOptions({
    queryKey: ["knowledge", "draft", id],
    queryFn: ({ signal }) => knowledgeApi.draft(id, signal),
    retry: false,
  });

export const recentOptions = () =>
  queryOptions({
    queryKey: ["knowledge", "recent"],
    queryFn: ({ signal }) => knowledgeApi.recent(signal),
    retry: false,
  });

export const listOptions = (params: KnowledgeListParams) =>
  queryOptions({
    queryKey: knowledgeQueryKeys.list(params),
    queryFn: ({ signal }) => knowledgeApi.list(params, signal),
    retry: false,
  });

export const folderOptions = () =>
  queryOptions({
    queryKey: knowledgeQueryKeys.folders(),
    queryFn: ({ signal }) => knowledgeApi.folders(signal),
    retry: false,
  });

export function useKnowledgeUpload() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: ({ file, folderId }: { file: File; folderId?: string }) =>
      knowledgeApi.upload(file, folderId),
    onSuccess: async () => {
      await client.invalidateQueries({ queryKey: knowledgeQueryKeys.all });
    },
  });
}

export function useFolderCreate() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: (input: FolderInput) => knowledgeApi.createFolder(input),
    onSuccess: async () => {
      await client.invalidateQueries({ queryKey: knowledgeQueryKeys.all });
    },
  });
}

export function useFolderUpdate() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: ({ id, version, input }: { id: string; version: number; input: FolderInput }) =>
      knowledgeApi.updateFolder(id, version, input),
    onSuccess: async () => {
      await client.invalidateQueries({ queryKey: knowledgeQueryKeys.all });
    },
  });
}

export function useFolderDelete() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: ({ id, version }: { id: string; version: number }) =>
      knowledgeApi.deleteFolder(id, version),
    onSuccess: async () => {
      await client.invalidateQueries({ queryKey: knowledgeQueryKeys.all });
    },
  });
}

export function useKnowledgeMove(id: string) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: ({ version, folderId }: { version: number; folderId: string | null }) =>
      knowledgeApi.move(id, version, folderId),
    onSuccess: async (detail) => {
      client.setQueryData(knowledgeQueryKeys.detail(id), detail);
      await client.invalidateQueries({ queryKey: knowledgeQueryKeys.all });
    },
  });
}

export function useKnowledgeSave(id: string) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: (input: SaveInput) => knowledgeApi.save(id, input),
    onSuccess: async (detail) => {
      client.setQueryData(knowledgeQueryKeys.detail(id), detail);
      await client.invalidateQueries({ queryKey: knowledgeQueryKeys.all, refetchType: "none" });
      await client.invalidateQueries({ queryKey: ["knowledge", "recent"] });
    },
  });
}

export function useDraftCommit(id: string) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: (input: SaveInput) => knowledgeApi.commit(id, input),
    onSuccess: async () => {
      await client.invalidateQueries({ queryKey: knowledgeQueryKeys.all, refetchType: "none" });
    },
  });
}

export function useVisibility(id: string) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: ({ version, visibility }: { version: number; visibility: Visibility }) =>
      knowledgeApi.visibility(id, version, visibility),
    onSuccess: async (detail) => {
      client.setQueryData(knowledgeQueryKeys.detail(id), detail);
      await client.invalidateQueries({ queryKey: knowledgeQueryKeys.all, refetchType: "none" });
      await client.invalidateQueries({ queryKey: ["knowledge", "recent"] });
    },
  });
}

// URL権限はQueryキャッシュや永続ストレージに置かず表示中のコンポーネントだけで保持する。
export const requestPreview = (id: string, version: number) => knowledgeApi.preview(id, version);

export const checkDraftResult = (id: string) => knowledgeApi.draft(id);
