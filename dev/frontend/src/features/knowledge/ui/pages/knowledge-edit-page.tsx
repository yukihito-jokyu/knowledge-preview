import { useRef, useState } from "react";
import { Link, useBlocker, useNavigate } from "@tanstack/react-router";
import { useQuery } from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import {
  AlertDialog,
  AlertDialogContent,
  AlertDialogTitle,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogCancel,
  AlertDialogAction,
} from "@/components/ui/alert-dialog";
import { MarkdownPreview } from "../components/markdown-preview";
import {
  checkDraftResult,
  detailOptions,
  draftOptions,
  recentOptions,
  useKnowledgeSave,
  useDraftCommit,
  useVisibility,
  knowledgeError,
  markdownBody,
  validateSource,
  parseViewMode,
  type ViewMode,
} from "../../model/knowledge-queries";
import type { EditableKnowledge, KnowledgeDetail } from "../../model/knowledge-types";
import { HtmlPreview } from "../components/html-preview";

export function KnowledgeEditPage({
  id,
  draft = false,
  mode = "preview",
}: {
  id: string;
  draft?: boolean;
  mode?: ViewMode;
}) {
  return draft ? (
    <DraftPage key={id} id={id} mode={mode} />
  ) : (
    <SavedPage key={id} id={id} mode={mode} />
  );
}

function SavedPage({ id, mode }: { id: string; mode: ViewMode }) {
  const query = useQuery(detailOptions(id));
  if (query.data)
    return <Editor key={id} id={id} initial={query.data} saved={query.data} initialMode={mode} />;
  return (
    <LoadState
      error={query.error}
      retry={() => {
        query.refetch().catch(() => {});
      }}
    />
  );
}

function DraftPage({ id, mode }: { id: string; mode: ViewMode }) {
  const query = useQuery(draftOptions(id));
  const [opened, setOpened] = useState<EditableKnowledge>();
  if (!opened && query.data?.status === "pending") setOpened(query.data);
  if (opened)
    return (
      <Editor
        key={id}
        id={id}
        draft
        initial={opened}
        initialMode={mode}
        committedId={query.data?.status === "committed" ? query.data.knowledgeId : undefined}
      />
    );
  if (query.data?.status === "committed")
    return (
      <section>
        <h1>この下書きは保存済みです</h1>
        <Link to="/knowledge/$knowledgeId/edit" params={{ knowledgeId: query.data.knowledgeId }}>
          保存済みの知識を開く
        </Link>
      </section>
    );
  if (query.data?.status === "pending")
    return <Editor key={id} id={id} draft initial={query.data} initialMode={mode} />;
  return (
    <LoadState
      error={query.error}
      retry={() => {
        query.refetch().catch(() => {});
      }}
    />
  );
}

function LoadState({ error, retry }: { error: unknown; retry: () => void }) {
  return error ? (
    <div role="alert">
      <p>{knowledgeError(error)}</p>
      <Button onClick={retry}>再試行</Button>
    </div>
  ) : (
    <p role="status">知識を読み込み中…</p>
  );
}

function Editor({
  id,
  draft = false,
  initial,
  saved: initialSaved,
  initialMode,
  committedId,
}: {
  id: string;
  draft?: boolean;
  initial: EditableKnowledge;
  saved?: KnowledgeDetail;
  initialMode: ViewMode;
  committedId?: string;
}) {
  // 編集セッションの保存版を固定し、背景再取得で本文・版・表示を混在させない。
  const [saved, setSaved] = useState(initialSaved);
  const [source, setSource] = useState(initial.source);
  const [baseline, setBaseline] = useState(initial.source);
  const [version, setVersion] = useState(initial.version);
  const [mode, setMode] = useState(parseViewMode(initialMode));
  const [notice, setNotice] = useState("");
  const [failure, setFailure] = useState("");
  const [search, setSearch] = useState("");
  const [searchError, setSearchError] = useState("");
  const [stopOpen, setStopOpen] = useState(false);
  const [recoveredId, setRecoveredId] = useState<string>();
  const navigatingAfterSave = useRef(false);
  const editor = useRef<HTMLTextAreaElement>(null);
  const save = useKnowledgeSave(id);
  const commit = useDraftCommit(id);
  const visibility = useVisibility(id);
  const recent = useQuery(recentOptions());
  const navigate = useNavigate();
  const dirty = source !== baseline;
  const pending = save.isPending || commit.isPending || visibility.isPending;

  const blocker = useBlocker({
    shouldBlockFn: () => !navigatingAfterSave.current && (dirty || pending),
    enableBeforeUnload: dirty || pending,
    withResolver: true,
  });

  const current = saved ?? initial;
  const recoveryTarget = recoveredId ?? committedId;

  function fail(error: unknown) {
    setFailure(knowledgeError(error));
  }

  function saveSource() {
    if (pending) return;
    const validation = validateSource(source);
    setFailure(validation ?? "");
    if (validation) {
      editor.current?.focus();
      return;
    }
    const submitted = source;
    if (draft) {
      commit.mutate(
        { version, source: submitted },
        {
          onSuccess: (result) => {
            setBaseline(submitted);
            navigatingAfterSave.current = true;
            navigate({
              to: "/knowledge/$knowledgeId/edit",
              params: { knowledgeId: result.knowledgeId },
              replace: true,
            }).catch((error: unknown) => {
              navigatingAfterSave.current = false;
              fail(error);
            });
          },
          onError: fail,
        },
      );
    } else {
      save.mutate(
        { version, source: submitted },
        {
          onSuccess: (result) => {
            setBaseline(submitted);
            setVersion(result.version);
            setSaved(result);
            setNotice("保存しました。");
          },
          onError: fail,
        },
      );
    }
  }

  function changeVisibility(next: "private" | "unlisted") {
    if (!saved || dirty || pending) return;
    setFailure("");
    visibility.mutate(
      { version, visibility: next },
      {
        onSuccess: (result) => {
          setVersion(result.version);
          setSaved(result);
          setNotice(next === "private" ? "公開を停止しました。" : "URL限定で公開しました。");
        },
        onError: fail,
      },
    );
  }

  function recoverDraft() {
    setFailure("");
    // 応答消失時の確認は保存の再送と分離し、編集中の入力を上書きしない。
    checkDraftResult(id)
      .then((result) => {
        if (result.status === "committed") setRecoveredId(result.knowledgeId);
        else setNotice("まだ正式保存されていません。同じ入力で再試行できます。");
      })
      .catch(fail);
  }

  return (
    <section
      className="overflow-hidden rounded-2xl border bg-card/90 shadow-raised"
      aria-label="知識閲覧・編集"
    >
      <div className="flex flex-wrap items-center gap-3 border-b p-4">
        <form
          className="flex min-w-56 flex-1 gap-2"
          onSubmit={(event) => {
            event.preventDefault();
            const q = search.trim();
            if (!q) return;
            if (Array.from(q).length > 200) {
              setSearchError("検索語は200文字以内にしてください。");
              return;
            }
            setSearchError("");
            navigate({ to: "/knowledge", search: { q, page: 1, pageSize: 10 } }).catch(fail);
          }}
        >
          <Input
            aria-label="知識を全文検索"
            placeholder="知識を全文検索…"
            value={search}
            onChange={(event) => setSearch(event.target.value)}
          />
          <Button variant="outline" disabled={pending}>
            検索
          </Button>
        </form>
        {saved?.visibility === "unlisted" && saved.publicId ? (
          <Button asChild variant="outline" disabled={pending}>
            <Link to="/public/knowledge/$publicId" params={{ publicId: saved.publicId }}>
              公開プレビュー
            </Link>
          </Button>
        ) : (
          <span className="text-sm text-muted-foreground">
            公開プレビューは保存・公開後に利用できます
          </span>
        )}
        {searchError && (
          <p role="alert" className="w-full text-destructive">
            {searchError}
          </p>
        )}
      </div>
      <div className="grid lg:grid-cols-[15rem_minmax(0,1fr)]">
        <aside className="border-b p-4 lg:border-r lg:border-b-0" aria-label="最近のファイル">
          <h2 className="mb-4 font-semibold">最近のファイル</h2>
          {recent.isPending && <p role="status">読み込み中…</p>}
          {recent.isError && (
            <div role="alert">
              <p>最近のファイルを取得できません。</p>
              <Button
                size="sm"
                variant="ghost"
                onClick={() => {
                  recent.refetch().catch(fail);
                }}
              >
                再試行
              </Button>
            </div>
          )}
          {recent.data?.length === 0 && (
            <p className="text-sm text-muted-foreground">保存済みファイルはありません。</p>
          )}
          <ul className="space-y-2">
            {recent.data?.map((item) => (
              <li key={item.id}>
                <Link
                  to="/knowledge/$knowledgeId/edit"
                  params={{ knowledgeId: item.id }}
                  aria-current={!draft && item.id === id ? "page" : undefined}
                  className="block rounded-xl p-3 text-sm hover:bg-accent aria-[current=page]:bg-secondary"
                >
                  <span className="block truncate font-medium">{item.title}</span>
                  <span className="text-xs text-muted-foreground">
                    {item.format === "html" ? "HTML" : "Markdown"} ·{" "}
                    {new Date(item.updatedAt).toLocaleDateString("ja-JP")}
                  </span>
                </Link>
              </li>
            ))}
          </ul>
        </aside>
        <div className="min-w-0 p-4 md:p-6">
          <nav aria-label="パンくず" className="mb-4 flex flex-wrap gap-2 text-sm">
            <Link to="/knowledge">マイ辞書</Link>
            <span>/</span>
            {current.folder && (
              <>
                <Link
                  to="/knowledge"
                  search={{ folderId: current.folder.id, page: 1, pageSize: 10 }}
                >
                  {current.folder.name}
                </Link>
                <span>/</span>
              </>
            )}
            <span aria-current="page">{current.title}</span>
          </nav>
          <div className="mb-4 flex flex-wrap items-center justify-between gap-3">
            <h1 className="text-xl font-semibold">{current.title}</h1>
            <div role="group" aria-label="表示モード" className="flex gap-1">
              {(
                [
                  ["preview", "プレビュー"],
                  ["editor", "エディター"],
                  ["split", "左右分割"],
                ] as const
              ).map(([value, label]) => (
                <Button
                  key={value}
                  size="sm"
                  variant={mode === value ? "secondary" : "ghost"}
                  aria-pressed={mode === value}
                  onClick={() => setMode(value)}
                >
                  {label}
                </Button>
              ))}
            </div>
          </div>
          <div className="mb-4 flex flex-wrap items-center gap-2">
            <Badge>{initial.format === "html" ? "HTML" : "Markdown"}</Badge>
            <span className="text-sm text-muted-foreground">
              {draft ? "下書き・正式保存前" : dirty ? "未保存の変更があります" : "保存済み"}
            </span>
            <div className="ml-auto flex gap-2">
              <Button disabled={pending || (!draft && !dirty)} onClick={saveSource}>
                {pending ? "処理中…" : draft ? "正式保存" : "保存"}
              </Button>
              {saved && (
                <Button
                  variant="outline"
                  disabled={pending || dirty}
                  onClick={() =>
                    saved.visibility === "unlisted"
                      ? setStopOpen(true)
                      : changeVisibility("unlisted")
                  }
                >
                  {saved.visibility === "unlisted" ? "公開停止" : "URL限定公開"}
                </Button>
              )}
            </div>
          </div>
          {dirty && (
            <p className="mb-3 text-sm text-muted-foreground">
              公開状態を変更する前に保存してください。HTMLのプレビューは保存成功後に更新します。
            </p>
          )}
          <p role="status" aria-live="polite" className="mb-2 text-sm">
            {notice}
            {saved?.warnings?.some((warning) => warning.code === "html_sanitized") && (
              <span className="block">安全のためHTMLの一部を除去して表示しています。</span>
            )}
          </p>
          {failure && (
            <div
              role="alert"
              className="mb-3 rounded-xl border border-destructive p-3 text-destructive"
            >
              {failure}
              {draft && (
                <Button variant="outline" onClick={recoverDraft}>
                  正式保存の結果を確認
                </Button>
              )}
            </div>
          )}
          {recoveryTarget && (
            <Link to="/knowledge/$knowledgeId/edit" params={{ knowledgeId: recoveryTarget }}>
              保存済みの知識を開く（未保存入力は破棄）
            </Link>
          )}
          <div
            className={`grid overflow-hidden rounded-xl border ${mode === "split" ? "xl:grid-cols-2" : ""}`}
          >
            <div hidden={mode === "preview"} className="min-w-0 border-r bg-muted/40">
              <label htmlFor="knowledge-source" className="block p-3 text-sm font-medium">
                原文
                {initial.format === "markdown"
                  ? "（title・tags・learningStatusはfront matterで編集）"
                  : ""}
              </label>
              <textarea
                ref={editor}
                id="knowledge-source"
                value={source}
                onChange={(event) => setSource(event.target.value)}
                readOnly={draft && pending}
                spellCheck={false}
                className="min-h-[32rem] w-full resize-y bg-transparent p-4 font-mono text-sm outline-none focus:ring-2 focus:ring-inset focus:ring-ring"
              />
            </div>
            <div hidden={mode === "editor"} className="min-w-0 p-5">
              <div className="mb-5 flex flex-wrap gap-2">
                {[...new Set(current.tags.map((tag) => tag.trim()).filter(Boolean))].map((tag) => (
                  <Link key={tag} to="/knowledge" search={{ tag: [tag], page: 1, pageSize: 10 }}>
                    <Badge variant="secondary"># {tag}</Badge>
                  </Link>
                ))}
                <Badge variant="outline">
                  {
                    { unlearned: "未学習", learning: "学習中", learned: "学習済み" }[
                      current.learningStatus
                    ]
                  }
                </Badge>
              </div>
              {initial.format === "markdown" ? (
                <MarkdownPreview source={markdownBody(source)} />
              ) : draft ? (
                <p>正式保存後にプレビューを表示します。</p>
              ) : (
                <HtmlPreview
                  key={`${id}:${saved?.version ?? version}`}
                  id={id}
                  version={saved?.version ?? version}
                />
              )}
            </div>
          </div>
        </div>
      </div>
      <AlertDialog
        open={blocker.status === "blocked"}
        onOpenChange={(open) => {
          if (!open) blocker.reset?.();
        }}
      >
        <AlertDialogContent onCloseAutoFocus={() => editor.current?.focus()}>
          <AlertDialogTitle>
            {pending ? "保存処理中です" : "未保存の変更を破棄しますか？"}
          </AlertDialogTitle>
          <AlertDialogDescription>
            {pending
              ? "処理が完了するまでこの画面でお待ちください。"
              : "移動すると編集中の入力は失われます。"}
          </AlertDialogDescription>
          <AlertDialogFooter>
            <AlertDialogCancel onClick={() => blocker.reset?.()}>編集を続ける</AlertDialogCancel>
            <AlertDialogAction disabled={pending} onClick={() => blocker.proceed?.()}>
              破棄して移動
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
      <AlertDialog open={stopOpen} onOpenChange={setStopOpen}>
        <AlertDialogContent>
          <AlertDialogTitle>公開を停止しますか？</AlertDialogTitle>
          <AlertDialogDescription>
            停止中は公開URLから閲覧できません。再公開では同じURLを使用します。
          </AlertDialogDescription>
          <AlertDialogFooter>
            <AlertDialogCancel>キャンセル</AlertDialogCancel>
            <AlertDialogAction onClick={() => changeVisibility("private")}>
              公開を停止
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </section>
  );
}
