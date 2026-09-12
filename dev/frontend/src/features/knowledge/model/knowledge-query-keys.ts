export const knowledgeQueryKeys = {
  all: ["knowledge"] as const,
  list: (search: string) => [...knowledgeQueryKeys.all, "list", { search }] as const,
  detail: (id: string) => [...knowledgeQueryKeys.all, "detail", id] as const,
};
