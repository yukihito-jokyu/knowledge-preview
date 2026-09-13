import { createFileRoute } from "@tanstack/react-router";
import { parseViewMode } from "@/features/knowledge/model/knowledge-editor";

export const Route = createFileRoute("/_authenticated/knowledge-drafts/$draftId/edit")({
  validateSearch: (search): { mode?: ReturnType<typeof parseViewMode> } => ({
    mode: parseViewMode(search.mode),
  }),
});
