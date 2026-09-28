import { httpClient } from "@/lib/http/client";
import type {
  PublicKnowledge,
  PublicKnowledgeContent,
  PublicKnowledgeFormat,
  PublicKnowledgeRelated,
} from "../model/public-knowledge";
import { isPublicId } from "../lib/public-id";

function record(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null;
}

function publicId(value: unknown): value is string {
  return typeof value === "string" && isPublicId(value);
}

function format(value: unknown): value is PublicKnowledgeFormat {
  return value === "markdown" || value === "html";
}

function strings(value: unknown): value is string[] {
  return Array.isArray(value) && value.every((item) => typeof item === "string");
}

function content(
  value: unknown,
  expectedFormat: PublicKnowledgeFormat,
  expectedPublicId: string,
  expectedVersion: number,
): value is PublicKnowledgeContent {
  if (!record(value)) return false;
  if (expectedFormat === "markdown")
    return typeof value.markdown === "string" && !("htmlUrl" in value);
  if (
    typeof value.htmlUrl !== "string" ||
    !/^https?:\/\//.test(value.htmlUrl) ||
    !("htmlUrl" in value)
  )
    return false;
  try {
    const url = new URL(value.htmlUrl);
    return (
      url.pathname === `/public/${encodeURIComponent(expectedPublicId)}/html` &&
      url.searchParams.get("version") === String(expectedVersion) &&
      !url.hash
    );
  } catch {
    return false;
  }
}

function parseRelated(value: unknown): PublicKnowledgeRelated | undefined {
  if (
    !record(value) ||
    !publicId(value.publicId) ||
    typeof value.title !== "string" ||
    !value.title.trim() ||
    !format(value.format) ||
    typeof value.summary !== "string"
  )
    return undefined;
  return {
    publicId: value.publicId,
    title: value.title,
    format: value.format,
    summary: value.summary,
  };
}

export function parsePublicKnowledge(value: unknown): PublicKnowledge {
  if (
    !record(value) ||
    !publicId(value.publicId) ||
    typeof value.title !== "string" ||
    !value.title.trim() ||
    !format(value.format) ||
    !strings(value.tags) ||
    typeof value.version !== "number" ||
    !Number.isSafeInteger(value.version) ||
    value.version < 1 ||
    typeof value.updatedAt !== "string" ||
    !Number.isFinite(Date.parse(value.updatedAt)) ||
    value.publicScope !== "unlisted" ||
    typeof value.summary !== "string" ||
    !content(value.content, value.format, value.publicId, value.version) ||
    !Array.isArray(value.related)
  )
    throw new Error("不正な公開知識応答です。");

  const related = value.related.map(parseRelated);

  return {
    publicId: value.publicId,
    title: value.title,
    format: value.format,
    tags: [...new Set(value.tags)],
    version: value.version,
    updatedAt: value.updatedAt,
    publicScope: "unlisted",
    summary: value.summary,
    content: value.content,
    related: related
      .filter((item): item is PublicKnowledgeRelated => item !== undefined)
      .filter(
        (item, index, items) =>
          item.publicId !== value.publicId &&
          items.findIndex((other) => other.publicId === item.publicId) === index,
      )
      .slice(0, 3),
  };
}

export const publicKnowledgeApi = {
  get: async (publicId: string, signal?: AbortSignal) =>
    parsePublicKnowledge(
      await httpClient.get<unknown>(`/public/knowledge/${encodeURIComponent(publicId)}`, {
        signal,
      }),
    ),
};
