import { createLazyFileRoute } from "@tanstack/react-router";
import { PublicKnowledgePage } from "@/features/public-knowledge/ui/pages/public-knowledge-page";

export const Route = createLazyFileRoute("/public/knowledge/$publicId")({
  component: PublicKnowledgeRoute,
});

function PublicKnowledgeRoute() {
  const { publicId } = Route.useParams();
  return <PublicKnowledgePage publicId={publicId} />;
}
