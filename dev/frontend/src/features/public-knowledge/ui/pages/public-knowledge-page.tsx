import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { MarkdownRenderer } from "@/lib/markdown/markdown-renderer";
import {
  isPublicId,
  isPublicKnowledgeRetryable,
  isPublicKnowledgeUnavailable,
  markdownHeadings,
  publicKnowledgeOptions,
  publicKnowledgeUrl,
  safePublicHtmlUrl,
} from "../../model/public-knowledge";
import type { PublicKnowledge } from "../../model/public-knowledge";

export function PublicKnowledgePage({ publicId }: { publicId: string }) {
  const query = useQuery(publicKnowledgeOptions(publicId));
  const refetch = query.refetch;

  if (!isPublicId(publicId)) return <UnavailableState />;
  if (query.isPending) return <LoadingState />;
  if (query.isFetching && query.data) return <LoadingState />;
  if (query.error) {
    if (isPublicKnowledgeUnavailable(query.error)) return <UnavailableState />;
    if (isPublicKnowledgeRetryable(query.error))
      return (
        <RetryState
          retry={() => {
            refetch().catch(() => {});
          }}
          pending={query.isFetching}
        />
      );
    return <UnavailableState />;
  }
  if (!query.data)
    return (
      <RetryState
        retry={() => {
          refetch().catch(() => {});
        }}
        pending={false}
      />
    );
  return <PublicKnowledgeView knowledge={query.data} />;
}

function LoadingState() {
  return (
    <main className="mx-auto flex min-h-[60vh] max-w-5xl items-center justify-center px-4 py-12">
      <p role="status" aria-live="polite">
        公開知識を読み込んでいます…
      </p>
    </main>
  );
}

function UnavailableState() {
  return (
    <main className="mx-auto flex min-h-[60vh] max-w-5xl items-center justify-center px-4 py-12">
      <section className="rounded-2xl border border-border/80 bg-card/85 p-8 text-center shadow-soft">
        <h1 className="text-2xl font-bold">この知識は閲覧できません</h1>
        <p className="mt-2 text-muted-foreground">公開されていないか、URLが無効です。</p>
      </section>
    </main>
  );
}

function RetryState({ retry, pending }: { retry: () => void; pending: boolean }) {
  return (
    <main className="mx-auto flex min-h-[60vh] max-w-5xl items-center justify-center px-4 py-12">
      <section
        role="alert"
        className="rounded-2xl border border-destructive-border bg-card/85 p-8 text-center shadow-soft"
      >
        <h1 className="text-2xl font-bold">読み込みに失敗しました</h1>
        <p className="mt-2 text-muted-foreground">時間をおいて、もう一度お試しください。</p>
        <Button className="mt-5" onClick={retry} disabled={pending}>
          {pending ? "再試行中…" : "再試行"}
        </Button>
      </section>
    </main>
  );
}

function PublicKnowledgeView({ knowledge }: { knowledge: PublicKnowledge }) {
  const shareUrl = publicKnowledgeUrl(knowledge.publicId, window.location.href);
  const [shareMessage, setShareMessage] = useState("");

  const markdown =
    knowledge.format === "markdown" && "markdown" in knowledge.content
      ? knowledge.content.markdown
      : undefined;

  const headings = markdown === undefined ? [] : markdownHeadings(markdown);

  const htmlUrl =
    knowledge.format === "html" && "htmlUrl" in knowledge.content
      ? safePublicHtmlUrl(
          knowledge.content.htmlUrl,
          knowledge.publicId,
          knowledge.version,
          window.location.href,
        )
      : undefined;

  function copyShareUrl() {
    if (!shareUrl || !navigator.clipboard) {
      setShareMessage("共有URLをコピーできません。URLを選択してコピーしてください。");
      return;
    }
    navigator.clipboard.writeText(shareUrl).then(
      () => setShareMessage("共有URLをコピーしました。"),
      () => setShareMessage("共有URLをコピーできません。URLを選択してコピーしてください。"),
    );
  }

  if (knowledge.format === "html" && !htmlUrl)
    return <RetryState retry={() => window.location.reload()} pending={false} />;

  return (
    <div className="min-h-screen">
      <header className="border-b border-border/70 bg-card/70 backdrop-blur-xl">
        <div className="mx-auto flex max-w-6xl flex-wrap items-center justify-between gap-4 px-4 py-4 sm:px-6">
          <Link to="/" className="text-lg font-bold text-primary">
            Knowledge Preview
          </Link>
          {shareUrl && (
            <div className="flex min-w-0 flex-1 flex-wrap items-center justify-end gap-2 sm:flex-nowrap">
              <label className="sr-only" htmlFor="public-share-url">
                共有URL
              </label>
              <Input
                id="public-share-url"
                aria-label="共有URL"
                className="min-w-0 flex-1 sm:max-w-md"
                readOnly
                value={shareUrl}
                onFocus={(event) => event.currentTarget.select()}
              />
              <Button type="button" variant="outline" onClick={copyShareUrl}>
                共有
              </Button>
            </div>
          )}
        </div>
        {shareMessage && (
          <p
            className="mx-auto max-w-6xl px-4 pb-3 text-right text-sm text-muted-foreground sm:px-6"
            role={shareMessage.includes("コピーしました") ? "status" : "alert"}
            aria-live="polite"
          >
            {shareMessage}
          </p>
        )}
      </header>

      <main className="mx-auto max-w-6xl px-4 py-8 sm:px-6 sm:py-12">
        <article>
          <header className="mb-8 border-b border-border/70 pb-6">
            <div className="flex flex-wrap items-center gap-2 text-sm text-muted-foreground">
              <Badge variant="secondary">限定公開</Badge>
              <time dateTime={knowledge.updatedAt}>{formatUpdatedAt(knowledge.updatedAt)}</time>
            </div>
            <h1 className="mt-4 text-3xl font-bold tracking-tight sm:text-4xl">
              {knowledge.title}
            </h1>
            {knowledge.tags.length > 0 && (
              <ul className="mt-4 flex flex-wrap gap-2" aria-label="タグ">
                {knowledge.tags.map((tag) => (
                  <li key={tag}>
                    <Badge variant="outline">#{tag}</Badge>
                  </li>
                ))}
              </ul>
            )}
            {knowledge.summary && (
              <p className="mt-5 max-w-3xl text-lg leading-8 text-muted-foreground">
                {knowledge.summary}
              </p>
            )}
          </header>

          <div className="grid min-w-0 gap-10 lg:grid-cols-[minmax(0,1fr)_15rem]">
            <div className="min-w-0">
              {markdown !== undefined ? (
                <MarkdownContent source={markdown} />
              ) : (
                <iframe
                  title="公開HTML本文"
                  src={htmlUrl}
                  sandbox=""
                  referrerPolicy="no-referrer"
                  className="min-h-[32rem] w-full border-0"
                />
              )}
            </div>
            {knowledge.format === "markdown" && headings.length > 0 && (
              <nav aria-label="このページ" className="min-w-0 self-start lg:sticky lg:top-6">
                <h2 className="mb-3 text-lg font-semibold">このページ</h2>
                <ol className="space-y-2 text-sm">
                  {headings.map((heading) => (
                    <li key={heading.id} className={heading.level > 1 ? "pl-3" : undefined}>
                      <a
                        className="text-muted-foreground hover:text-primary"
                        href={`#${heading.id}`}
                      >
                        {heading.title}
                      </a>
                    </li>
                  ))}
                </ol>
              </nav>
            )}
          </div>
        </article>

        {knowledge.related.length > 0 && (
          <section
            className="mt-12 border-t border-border/70 pt-8"
            aria-labelledby="related-knowledge"
          >
            <h2 id="related-knowledge" className="text-xl font-bold">
              関連記事
            </h2>
            <ul className="mt-4 grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
              {knowledge.related.map((item) => (
                <li
                  key={item.publicId}
                  className="rounded-2xl border border-border/80 bg-card/85 p-5 shadow-soft"
                >
                  <Link
                    to="/public/knowledge/$publicId"
                    params={{ publicId: item.publicId }}
                    className="font-semibold text-primary hover:underline"
                  >
                    {item.title}
                  </Link>
                  {item.summary && (
                    <p className="mt-2 text-sm text-muted-foreground">{item.summary}</p>
                  )}
                </li>
              ))}
            </ul>
          </section>
        )}
      </main>

      <footer className="border-t border-border/70 px-4 py-8 text-center text-sm text-muted-foreground">
        Knowledge Preview · 公開された知識のみを表示しています
      </footer>
    </div>
  );
}

function MarkdownContent({ source }: { source: string }) {
  return (
    <div className="knowledge-markdown min-w-0">
      <MarkdownRenderer
        components={{
          h1: ({ node, children }) => (
            <h1 id={`heading-${node?.position?.start.line}`}>{children}</h1>
          ),
          h2: ({ node, children }) => (
            <h2 id={`heading-${node?.position?.start.line}`}>{children}</h2>
          ),
          h3: ({ node, children }) => (
            <h3 id={`heading-${node?.position?.start.line}`}>{children}</h3>
          ),
          blockquote: ({ children }) => (
            <blockquote className="border-l-4 border-primary/50 bg-primary/5 py-2">
              {children}
            </blockquote>
          ),
        }}
      >
        {source}
      </MarkdownRenderer>
    </div>
  );
}

function formatUpdatedAt(value: string): string {
  return new Intl.DateTimeFormat("ja-JP", { dateStyle: "medium" }).format(new Date(value));
}
