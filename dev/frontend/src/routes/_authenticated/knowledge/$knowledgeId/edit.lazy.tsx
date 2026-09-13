import { createLazyFileRoute } from "@tanstack/react-router";
import { KnowledgeEditPage } from "@/features/knowledge/ui/pages/knowledge-edit-page";

export const Route = createLazyFileRoute("/_authenticated/knowledge/$knowledgeId/edit")({
  component: EditRoute,
});

function EditRoute() {
  const { knowledgeId } = Route.useParams();
  const { mode } = Route.useSearch();
  return <KnowledgeEditPage id={knowledgeId} mode={mode} />;
}
