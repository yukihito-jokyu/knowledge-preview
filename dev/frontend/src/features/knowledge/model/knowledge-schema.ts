export type KnowledgeInput = { title: string; markdown: string };

export function validateKnowledgeInput(input: KnowledgeInput): string[] {
  return input.title.trim() ? [] : ["タイトルは必須です。"];
}
