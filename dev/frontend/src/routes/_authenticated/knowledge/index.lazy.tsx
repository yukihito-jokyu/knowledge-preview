import { createLazyFileRoute } from "@tanstack/react-router";
import { KnowledgeListPage } from "@/features/knowledge/ui/pages/knowledge-list-page";

export const Route = createLazyFileRoute("/_authenticated/knowledge/")({
  component: KnowledgeListRoute,
});

function KnowledgeListRoute() {
  const search = Route.useSearch();
  return <KnowledgeListPage key={search.q ?? ""} search={search} />;
}
