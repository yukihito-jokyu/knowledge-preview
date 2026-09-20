export const knowledgeQueryKeys = {
  all: ["knowledge"] as const,
  list: (search: unknown) => [...knowledgeQueryKeys.all, "list", search] as const,
  detail: (id: string) => [...knowledgeQueryKeys.all, "detail", id] as const,
  folders: () => [...knowledgeQueryKeys.all, "folders"] as const,
};
