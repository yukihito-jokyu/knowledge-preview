export type Format = "markdown" | "html";
export type Visibility = "private" | "unlisted";
export type KnowledgeSummary = { id: string; title: string; format: Format; updatedAt: string };
export type EditableKnowledge = {
  title: string;
  format: Format;
  source: string;
  tags: string[];
  learningStatus: "unlearned" | "learning" | "learned";
  folder: { id: string; name: string } | null;
  version: number;
};
export type KnowledgeDetail = EditableKnowledge &
  KnowledgeSummary & {
    visibility: Visibility;
    publicId: string | null;
    publicUrl: string | null;
    warnings?: { code: string }[];
  };
export type DraftDetail =
  | (EditableKnowledge & { draftId: string; status: "pending" })
  | { draftId: string; status: "committed"; knowledgeId: string; editPath: string };
export type SaveInput = { version: number; source: string };
export type CommitResult = { knowledgeId: string; editPath: string };
