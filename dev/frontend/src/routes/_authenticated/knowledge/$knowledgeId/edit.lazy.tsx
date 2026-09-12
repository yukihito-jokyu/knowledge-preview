import { createLazyFileRoute } from "@tanstack/react-router";
import { KnowledgeEditPage } from "@/features/knowledge/ui/pages/knowledge-edit-page";

export const Route = createLazyFileRoute("/_authenticated/knowledge/$knowledgeId/edit")({
  component: KnowledgeEditPage,
});
