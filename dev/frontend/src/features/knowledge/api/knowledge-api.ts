import { httpClient } from "@/lib/http/client";
import type {
  CommitResult,
  DraftDetail,
  EditableKnowledge,
  KnowledgeDetail,
  KnowledgeSummary,
  SaveInput,
  Visibility,
} from "../model/knowledge-types";

const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;

function record(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null;
}

function isSummary(value: unknown): value is KnowledgeSummary & Record<string, unknown> {
  return (
    record(value) &&
    typeof value.id === "string" &&
    uuid.test(value.id) &&
    typeof value.title === "string" &&
    value.title.trim().length > 0 &&
    (value.format === "markdown" || value.format === "html") &&
    typeof value.updatedAt === "string" &&
    Number.isFinite(Date.parse(value.updatedAt))
  );
}

function isEditable(value: unknown): value is EditableKnowledge & Record<string, unknown> {
  return (
    record(value) &&
    typeof value.title === "string" &&
    typeof value.source === "string" &&
    (value.format === "markdown" || value.format === "html") &&
    Array.isArray(value.tags) &&
    value.tags.every((tag: unknown) => typeof tag === "string") &&
    ["unlearned", "learning", "learned"].includes(String(value.learningStatus)) &&
    typeof value.version === "number" &&
    Number.isSafeInteger(value.version) &&
    value.version > 0 &&
    (value.folder === null ||
      (record(value.folder) &&
        typeof value.folder.id === "string" &&
        uuid.test(value.folder.id) &&
        typeof value.folder.name === "string"))
  );
}

export function parseDetail(value: unknown, expectedId?: string): KnowledgeDetail {
  if (
    !isSummary(value) ||
    (expectedId !== undefined && value.id !== expectedId) ||
    !isEditable(value) ||
    !record(value) ||
    (value.visibility !== "private" && value.visibility !== "unlisted") ||
    !(value.publicId === null || typeof value.publicId === "string") ||
    !(value.publicUrl === null || typeof value.publicUrl === "string")
  )
    throw new Error("不正な知識応答です。");

  const warnings = Array.isArray(value.warnings)
    ? value.warnings.filter(
        (warning): warning is { code: string } =>
          record(warning) && typeof warning.code === "string",
      )
    : undefined;

  return {
    ...value,
    visibility: value.visibility,
    publicId: value.publicId,
    publicUrl: value.publicUrl,
    warnings,
  };
}

export function parseRecent(value: unknown): KnowledgeSummary[] {
  if (!record(value) || !Array.isArray(value.items))
    throw new Error("不正な最近のファイル応答です。");
  return value.items
    .filter(isSummary)
    .filter((item, index, items) => items.findIndex((other) => other.id === item.id) === index)
    .slice(0, 10);
}

function parseCommit(value: unknown): CommitResult {
  if (!record(value) || typeof value.knowledgeId !== "string" || !uuid.test(value.knowledgeId))
    throw new Error("不正な保存応答です。");
  return { knowledgeId: value.knowledgeId, editPath: `/knowledge/${value.knowledgeId}/edit` };
}

function parseDraft(value: unknown, expectedId: string): DraftDetail {
  if (
    !record(value) ||
    typeof value.draftId !== "string" ||
    !uuid.test(value.draftId) ||
    value.draftId !== expectedId
  )
    throw new Error("不正な下書き応答です。");
  if (value.status === "committed")
    return { draftId: value.draftId, status: "committed", ...parseCommit(value) };
  if (value.status !== "pending" || !isEditable(value)) throw new Error("不正な下書き応答です。");
  return { ...value, draftId: value.draftId, status: "pending" };
}

export const knowledgeApi = {
  detail: async (id: string, signal?: AbortSignal) =>
    parseDetail(
      await httpClient.get<unknown>(`/knowledge/${encodeURIComponent(id)}`, { signal }),
      id,
    ),
  recent: async (signal?: AbortSignal) =>
    parseRecent(await httpClient.get<unknown>("/knowledge/recent", { signal })),
  draft: async (id: string, signal?: AbortSignal) =>
    parseDraft(
      await httpClient.get<unknown>(`/knowledge-drafts/${encodeURIComponent(id)}`, { signal }),
      id,
    ),
  save: async (id: string, input: SaveInput) =>
    parseDetail(
      await httpClient.put<unknown, SaveInput>(`/knowledge/${encodeURIComponent(id)}`, input),
      id,
    ),
  commit: async (id: string, input: SaveInput) =>
    parseCommit(
      await httpClient.post<unknown, SaveInput>(
        `/knowledge-drafts/${encodeURIComponent(id)}/commit`,
        input,
      ),
    ),
  visibility: async (id: string, version: number, visibility: Visibility) =>
    parseDetail(
      await httpClient.put<unknown, { version: number; visibility: Visibility }>(
        `/knowledge/${encodeURIComponent(id)}/visibility`,
        { version, visibility },
      ),
      id,
    ),
  preview: async (id: string, version: number) => {
    const value = await httpClient.post<unknown, { version: number }>(
      `/knowledge/${encodeURIComponent(id)}/preview-ticket`,
      { version },
    );

    if (!record(value) || typeof value.url !== "string")
      throw new Error("不正なプレビュー応答です。");
    return value.url;
  },
};
