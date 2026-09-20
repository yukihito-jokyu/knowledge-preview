import { createFileRoute } from "@tanstack/react-router";

export const Route = createFileRoute("/_authenticated/knowledge/")({
  validateSearch: (
    search: Record<string, unknown>,
  ): { q?: string; tag?: string[]; folderId?: string; page?: number; pageSize?: number } => ({
    q: typeof search.q === "string" ? search.q.trim() : undefined,
    tag: Array.isArray(search.tag)
      ? search.tag.filter((tag): tag is string => typeof tag === "string")
      : typeof search.tag === "string"
        ? [search.tag]
        : undefined,
    folderId: typeof search.folderId === "string" ? search.folderId : undefined,
    page:
      typeof search.page === "number" && Number.isSafeInteger(search.page) && search.page > 0
        ? search.page
        : 1,
    pageSize:
      typeof search.pageSize === "number" && [10, 25, 50, 100].includes(search.pageSize)
        ? search.pageSize
        : 10,
  }),
});
