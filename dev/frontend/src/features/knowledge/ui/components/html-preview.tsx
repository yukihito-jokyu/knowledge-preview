import { useEffect, useState } from "react";
import { Button } from "@/components/ui/button";
import { requestPreview, safePreviewUrl } from "../../model/knowledge-queries";

export function HtmlPreview({ id, version }: { id: string; version: number }) {
  const [url, setUrl] = useState<string>();
  const [error, setError] = useState(false);
  const [attempt, setAttempt] = useState(0);
  useEffect(() => {
    let active = true;
    requestPreview(id, version)
      .then((value) => {
        if (!active) return;
        const safe = safePreviewUrl(value, window.location.href);
        setUrl(safe);
        setError(!safe);
      })
      .catch(() => {
        if (active) setError(true);
      });
    return () => {
      active = false;
    };
  }, [id, version, attempt]);
  if (error)
    return (
      <div role="alert">
        プレビューを取得できません。
        <Button
          variant="outline"
          onClick={() => {
            setUrl(undefined);
            setError(false);
            setAttempt(attempt + 1);
          }}
        >
          プレビューを再取得
        </Button>
      </div>
    );
  if (!url) return <p role="status">プレビューを読み込み中…</p>;
  return (
    <iframe
      title="保存済みHTMLプレビュー"
      src={url}
      sandbox=""
      referrerPolicy="no-referrer"
      className="min-h-[32rem] w-full border-0"
    />
  );
}
