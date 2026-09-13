import { createLazyFileRoute } from "@tanstack/react-router";
import { KnowledgeEditPage } from "@/features/knowledge/ui/pages/knowledge-edit-page";

export const Route = createLazyFileRoute("/_authenticated/knowledge-drafts/$draftId/edit")({
  component: DraftRoute,
});

function DraftRoute() {
  const { draftId } = Route.useParams();
  const { mode } = Route.useSearch();
  return <KnowledgeEditPage id={draftId} mode={mode} draft />;
}
