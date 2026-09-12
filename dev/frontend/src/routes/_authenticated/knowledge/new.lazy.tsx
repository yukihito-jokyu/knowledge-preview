import { createLazyFileRoute } from "@tanstack/react-router";
import { KnowledgeCreatePage } from "@/features/knowledge/ui/pages/knowledge-create-page";

export const Route = createLazyFileRoute("/_authenticated/knowledge/new")({
  component: KnowledgeCreatePage,
});
