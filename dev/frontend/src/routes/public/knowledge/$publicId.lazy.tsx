import { createLazyFileRoute } from "@tanstack/react-router";
import { PublicKnowledgePage } from "@/features/public-knowledge/ui/pages/public-knowledge-page";

export const Route = createLazyFileRoute("/public/knowledge/$publicId")({
  component: PublicKnowledgePage,
});
