import { queryOptions, useMutation, useQueryClient } from "@tanstack/react-query";
import { knowledgeApi } from "../api/knowledge-api";
import { knowledgeQueryKeys } from "./knowledge-query-keys";
import type { SaveInput, Visibility } from "./knowledge-types";
export {
  knowledgeError,
  markdownBody,
  parseViewMode,
  safePreviewUrl,
  validateSource,
} from "./knowledge-editor";
export type { ViewMode } from "./knowledge-editor";

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
