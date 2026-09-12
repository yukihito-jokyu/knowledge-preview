import { createLazyFileRoute } from "@tanstack/react-router";
import { KnowledgeListPage } from "@/features/knowledge/ui/pages/knowledge-list-page";

export const Route = createLazyFileRoute("/_authenticated/knowledge/")({
  component: KnowledgeListPage,
});
