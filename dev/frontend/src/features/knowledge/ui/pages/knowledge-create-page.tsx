import { useRef, useState } from "react";
import { Link, useNavigate } from "@tanstack/react-router";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import {
  useKnowledgeUpload,
  uploadError,
  validateUploadFiles,
} from "../../model/knowledge-queries";

export function KnowledgeCreatePage() {
  const input = useRef<HTMLInputElement>(null);
  const [error, setError] = useState("");
  const [dragging, setDragging] = useState(false);
  const upload = useKnowledgeUpload();
  const navigate = useNavigate();

  function reset() {
    setError("");
    if (input.current) input.current.value = "";
    input.current?.focus();
  }

  function choose(files: FileList | File[]) {
    if (upload.isPending) return;
    const validation = validateUploadFiles(files);
    if (validation) {
      setError(validation);
      return;
    }

    const file = files[0];
    setError("");
    upload.mutate(
      { file },
      {
        onSuccess: (created) => {
          navigate({
            to: "/knowledge-drafts/$draftId/edit",
            params: { draftId: created.draftId },
            replace: true,
          }).catch((reason: unknown) => setError(uploadError(reason)));
        },
        onError: (reason: unknown) => setError(uploadError(reason)),
      },
    );
  }

  return (
    <section className="mx-auto max-w-3xl" aria-labelledby="knowledge-create-title">
      <Card>
        <CardHeader>
          <CardTitle id="knowledge-create-title">知識を作成</CardTitle>
          <CardDescription>
            MarkdownまたはHTMLを1ファイル選択してください。解析後、確認・編集画面へ移動します。
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-5">
          {error && (
            <Alert variant="destructive">
              <AlertDescription>{error}</AlertDescription>
              <Button type="button" variant="outline" className="mt-3" onClick={reset}>
                別のファイルを選ぶ
              </Button>
            </Alert>
          )}
          <label
            htmlFor="knowledge-file"
            className={`block cursor-pointer rounded-2xl border-2 border-dashed p-10 text-center transition-colors focus-within:ring-4 focus-within:ring-ring/20 ${dragging ? "border-primary bg-primary/5" : "border-border hover:border-primary/60"} ${upload.isPending ? "pointer-events-none opacity-60" : ""}`}
            onDragEnter={(event) => {
              event.preventDefault();
              if (!upload.isPending) setDragging(true);
            }}
            onDragOver={(event) => event.preventDefault()}
            onDragLeave={() => setDragging(false)}
            onDrop={(event) => {
              event.preventDefault();
              setDragging(false);
              choose(event.dataTransfer.files);
            }}
          >
            <span className="block font-semibold">ここにファイルをドロップ</span>
            <span className="mt-2 block text-sm text-muted-foreground">またはクリックして選択</span>
            <span className="mt-4 block text-xs text-muted-foreground">
              対応形式: .md / .html · 1ファイル · 最大10 MiB
            </span>
            <input
              ref={input}
              id="knowledge-file"
              className="sr-only"
              type="file"
              accept=".md,.html"
              disabled={upload.isPending}
              onChange={(event) => {
                if (event.currentTarget.files) choose(event.currentTarget.files);
              }}
            />
          </label>
          <p role="status" aria-live="polite" aria-busy={upload.isPending}>
            {upload.isPending ? "解析中…" : ""}
          </p>
          <div className="flex flex-wrap gap-3 text-sm text-muted-foreground">
            <span>1. ファイルを選択</span>
            <span>2. 内容を解析</span>
            <span>3. 確認・編集</span>
          </div>
          <Button asChild variant="outline">
            <Link to="/knowledge">マイ辞書へ戻る</Link>
          </Button>
        </CardContent>
      </Card>
    </section>
  );
}
