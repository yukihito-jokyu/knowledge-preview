import { expect, test } from "vitest";
import { ApiError } from "@/lib/http/api-error";
import {
  isPublicId,
  isPublicKnowledgeRetryable,
  isPublicKnowledgeUnavailable,
  markdownHeadings,
  publicKnowledgeUrl,
  publicKnowledgeOptions,
  safePublicHtmlUrl,
} from "./public-knowledge";

const publicId = "A".repeat(43);

test("publicIdは32 bytesのbase64urlだけを通す", () => {
  expect(isPublicId(publicId)).toBe(true);
  expect(isPublicId("A".repeat(42))).toBe(false);
  expect(isPublicId(`${"A".repeat(42)}!`)).toBe(false);
});

test("共有URLとHTML URLは同一origin・公開routeの契約へ制限する", () => {
  expect(publicKnowledgeUrl(publicId, "https://app.example/public/knowledge/x")).toBe(
    `https://app.example/public/knowledge/${publicId}`,
  );
  expect(
    safePublicHtmlUrl(
      `https://preview.example/public/${publicId}/html?version=2`,
      publicId,
      2,
      "https://app.example/public/knowledge/x",
    ),
  ).toContain("version=2");
  expect(
    safePublicHtmlUrl(
      `https://app.example/public/${publicId}/html?version=2`,
      publicId,
      2,
      "https://app.example/public/knowledge/x",
    ),
  ).toBeUndefined();
});

test("目次はfenced code外のh1-h3だけを行番号IDへ変換する", () => {
  const source = "# 見出し\n```md\n## code\n```\n> 要点\n### 詳細";
  expect(markdownHeadings(source)).toEqual([
    { id: "heading-1", title: "見出し", level: 1 },
    { id: "heading-6", title: "詳細", level: 3 },
  ]);
});

test("404は閲覧不可、それ以外の通信・サーバー失敗は再試行にする", () => {
  expect(isPublicKnowledgeUnavailable(new ApiError("not found", 404))).toBe(true);
  expect(isPublicKnowledgeRetryable(new ApiError("unavailable", 503))).toBe(true);
  expect(isPublicKnowledgeRetryable(new ApiError("bad request", 400))).toBe(false);
});

test("公開queryは停止直後の再取得を優先し、暗黙のretryをしない", () => {
  const options = publicKnowledgeOptions(publicId);
  expect(options.staleTime).toBe(0);
  expect(options.retry).toBe(false);
});
