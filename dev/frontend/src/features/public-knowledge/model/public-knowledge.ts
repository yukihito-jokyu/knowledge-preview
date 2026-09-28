import { queryOptions } from "@tanstack/react-query";
import { ApiError } from "@/lib/http/api-error";
import { publicKnowledgeApi } from "../api/public-knowledge-api";
import { isPublicId } from "../lib/public-id";

export { isPublicId } from "../lib/public-id";

export type PublicKnowledgeFormat = "markdown" | "html";

export type PublicKnowledgeContent = { markdown: string } | { htmlUrl: string };

export type PublicKnowledgeRelated = {
  publicId: string;
  title: string;
  format: PublicKnowledgeFormat;
  summary: string;
};

export type PublicKnowledge = {
  publicId: string;
  title: string;
  format: PublicKnowledgeFormat;
  tags: string[];
  version: number;
  updatedAt: string;
  publicScope: "unlisted";
  summary: string;
  content: PublicKnowledgeContent;
  related: PublicKnowledgeRelated[];
};

export const publicKnowledgeQueryKeys = {
  detail: (publicId: string) => ["public-knowledge", publicId] as const,
};

export function publicKnowledgeUrl(publicId: string, applicationUrl: string): string | undefined {
  if (!isPublicId(publicId)) return undefined;
  try {
    const application = new URL(applicationUrl);
    const url = new URL(`/public/knowledge/${encodeURIComponent(publicId)}`, application.origin);
    return url.origin === application.origin ? url.href : undefined;
  } catch {
    return undefined;
  }
}

export function safePublicHtmlUrl(
  value: string,
  publicId: string,
  version: number,
  applicationUrl: string,
): string | undefined {
  try {
    const url = new URL(value);
    const application = new URL(applicationUrl);
    if (
      !["https:", "http:"].includes(url.protocol) ||
      url.hostname === application.hostname ||
      url.username ||
      url.password ||
      url.pathname !== `/public/${encodeURIComponent(publicId)}/html` ||
      url.searchParams.size !== 1 ||
      url.searchParams.get("version") !== String(version) ||
      url.hash
    )
      return undefined;
    if (application.protocol === "https:" && url.protocol !== "https:") return undefined;
    return url.href;
  } catch {
    return undefined;
  }
}

export function publicKnowledgeOptions(publicId: string) {
  return queryOptions({
    queryKey: publicKnowledgeQueryKeys.detail(publicId),
    queryFn: ({ signal }) => publicKnowledgeApi.get(publicId, signal),
    enabled: isPublicId(publicId),
    staleTime: 0,
    retry: false,
  });
}

export function isPublicKnowledgeUnavailable(error: unknown): boolean {
  return error instanceof ApiError && error.status === 404;
}

export function isPublicKnowledgeRetryable(error: unknown): boolean {
  if (!(error instanceof ApiError)) return true;
  return error.status === 0 || error.status >= 500;
}

export function markdownHeadings(source: string): { id: string; title: string; level: number }[] {
  let fence: string | undefined;

  return source.split(/\r?\n/).flatMap((line, index) => {
    const delimiter = /^\s{0,3}(`{3,}|~{3,})/.exec(line)?.[1];
    if (delimiter) {
      fence =
        fence && delimiter.startsWith(fence[0] ?? "") && delimiter.length >= fence.length
          ? undefined
          : (fence ?? delimiter);
      return [];
    }
    if (fence) return [];
    const heading = /^ {0,3}(#{1,3})\s+(.+?)\s*#*\s*$/.exec(line);
    return heading
      ? [{ id: `heading-${index + 1}`, title: heading[2] ?? "", level: heading[1]?.length ?? 1 }]
      : [];
  });
}
