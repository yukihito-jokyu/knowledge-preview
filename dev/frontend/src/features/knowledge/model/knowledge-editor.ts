import { ApiError } from "@/lib/http/api-error";
export type ViewMode = "preview" | "editor" | "split";

export const parseViewMode = (value: unknown): ViewMode =>
  value === "editor" || value === "split" ? value : "preview";

export function markdownBody(source: string): string {
  return source.replace(/^---\r?\n[\s\S]*?\r?\n---(?:\r?\n|$)/, "");
}

export function validateSource(source: string): string | undefined {
  if (!source.trim()) return "本文を入力してください。";
  if (new TextEncoder().encode(source).byteLength > 10 * 1024 * 1024)
    return "本文はUTF-8で10 MiB以内にしてください。";
  return undefined;
}

export function knowledgeError(error: unknown): string {
  if (!(error instanceof ApiError))
    return "通信に失敗しました。入力を保持しています。再試行してください。";
  if (error.status === 401)
    return "ログインの有効期限が切れました。入力を控えてから再ログインしてください。";
  if (error.status === 404) return "知識が見つからないか、閲覧権限がありません。";
  if (error.status === 409)
    return "別の操作で更新されています。入力を保持しています。保存済みの内容を確認してください。";
  if (error.status === 413) return "本文はUTF-8で10 MiB以内にしてください。";
  if (error.status === 422) {
    const position = error.details.find((detail) => detail.line !== undefined);
    return `本文またはfront matterを確認してください。${position ? ` ${position.line}行${position.column ?? 1}列` : ""}`;
  }
  return "保存できませんでした。入力を保持しています。再試行してください。";
}

export function safePreviewUrl(value: string, applicationUrl: string): string | undefined {
  try {
    const url = new URL(value);
    const app = new URL(applicationUrl);
    if (
      !["https:", "http:"].includes(url.protocol) ||
      url.hostname === app.hostname ||
      url.username ||
      url.password
    )
      return undefined;
    if (app.protocol === "https:" && url.protocol !== "https:") return undefined;
    return url.href;
  } catch {
    return undefined;
  }
}
