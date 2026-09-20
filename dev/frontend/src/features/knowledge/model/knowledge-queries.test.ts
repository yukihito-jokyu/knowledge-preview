import { expect, test } from "vitest";
import { ApiError } from "@/lib/http/api-error";
import {
  folderOperationError,
  validateSearchQuery,
  validateUploadFiles,
} from "./knowledge-queries";

function file(name: string, size: number) {
  return new File([new Uint8Array(size)], name);
}

test("upload入力は1件、拡張子、0 byte、10 MiB境界を検証する", () => {
  expect(validateUploadFiles([])).toContain("1件");
  expect(validateUploadFiles([file("note.txt", 1)])).toContain(".md");
  expect(validateUploadFiles([file("note.md", 0)])).toContain("空");
  expect(validateUploadFiles([file("note.md", 10 * 1024 * 1024)])).toBeUndefined();
  expect(validateUploadFiles([file("note.md", 10 * 1024 * 1024 + 1)])).toContain("10 MiB");
});

test("検索語はtrim後のUnicode code point数で200文字を上限にする", () => {
  expect(validateSearchQuery(`  ${"あ".repeat(200)}  `)).toBeUndefined();
  expect(validateSearchQuery("😀".repeat(201))).toContain("200文字");
});

test("folder操作は本文保存用ではなく操作別の固定エラーを返す", () => {
  expect(folderOperationError(new ApiError("invalid", 422), "create")).toContain(
    "フォルダ名または親フォルダ",
  );
  expect(folderOperationError(new ApiError("conflict", 409), "create")).toContain("同名");
  expect(folderOperationError(new ApiError("conflict", 409), "create")).not.toContain("再読み込み");
  expect(folderOperationError(new ApiError("conflict", 409), "update")).toContain("同名");
  expect(folderOperationError(new ApiError("conflict", 409), "update")).toContain("別の操作で更新");
  expect(folderOperationError(new ApiError("conflict", 409), "move")).toBe(
    "フォルダが別の操作で更新されています。再読み込みしてから再試行してください。",
  );
  expect(folderOperationError(new ApiError("conflict", 409), "delete")).toContain(
    "フォルダを削除できません",
  );
  expect(folderOperationError(new ApiError("conflict", 409), "delete")).not.toContain("保存");
});
