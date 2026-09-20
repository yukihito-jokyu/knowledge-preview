import { useCallback, useEffect, useRef, useState } from "react";
import { Link, useNavigate } from "@tanstack/react-router";
import { useQuery } from "@tanstack/react-query";
import { Alert, AlertDescription } from "@/components/ui/alert";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import {
  knowledgeError,
  folderOperationError,
  folderOptions,
  listOptions,
  validateSearchQuery,
  useFolderCreate,
  useFolderDelete,
  useFolderUpdate,
  useKnowledgeMove,
} from "../../model/knowledge-queries";
import type { Folder, KnowledgeListItem } from "../../model/knowledge-types";

type KnowledgeListSearch = {
  q?: string;
  tag?: string[];
  folderId?: string;
  page?: number;
  pageSize?: number;
};

export function KnowledgeListPage({ search }: { search: KnowledgeListSearch }) {
  const navigate = useNavigate();
  const page = search.page ?? 1;
  const pageSize = search.pageSize ?? 10;
  const query = search.q ?? "";
  const tags = search.tag ?? emptyTags;
  const [input, setInput] = useState(query);
  const [folderDialog, setFolderDialog] = useState<"create" | "rename">();
  const [folderName, setFolderName] = useState("");
  const [parentFolderId, setParentFolderId] = useState<string | null>(null);
  const [targetFolder, setTargetFolder] = useState<Folder>();
  const [deleteTarget, setDeleteTarget] = useState<Folder>();
  const [folderError, setFolderError] = useState("");
  const [searchError, setSearchError] = useState(() => validateSearchQuery(query) ?? "");
  const folderOpener = useRef<HTMLButtonElement | null>(null);
  const createFolderButton = useRef<HTMLButtonElement | null>(null);
  const folders = useQuery(folderOptions());

  const params = { query, tags, folderId: search.folderId, page, pageSize };

  const createFolder = useFolderCreate();
  const updateFolder = useFolderUpdate();
  const deleteFolder = useFolderDelete();
  const queryError = validateSearchQuery(query);

  const list = useQuery({
    ...listOptions(params),
    enabled: !queryError && !searchError,
  });

  const go = useCallback(
    (next: Partial<KnowledgeListSearch>) => {
      const nextQuery = "q" in next ? next.q : query;
      const nextTags = "tag" in next ? next.tag : tags;
      const nextFolder = "folderId" in next ? next.folderId : search.folderId;

      navigate({
        to: "/knowledge",
        search: {
          q: nextQuery || undefined,
          tag: nextTags?.length ? nextTags : undefined,
          folderId: nextFolder,
          page: next.page ?? 1,
          pageSize: next.pageSize ?? pageSize,
        },
      }).catch(() => undefined);
    },
    [navigate, pageSize, query, search.folderId, tags],
  );

  useEffect(() => {
    if (!list.data || list.data.page !== page || list.data.pageSize !== pageSize) return;
    const lastPage = Math.max(1, Math.ceil(list.data.total / pageSize));
    if (page > lastPage) go({ page: lastPage });
  }, [go, list.data, page, pageSize]);

  function saveFolder() {
    const name = folderName.trim();
    if (!name || createFolder.isPending || updateFolder.isPending) return;
    setFolderError("");

    const close = () => {
      setFolderDialog(undefined);
      setFolderName("");
      setTargetFolder(undefined);
      setParentFolderId(null);
    };

    if (folderDialog === "rename" && targetFolder)
      updateFolder.mutate(
        {
          id: targetFolder.id,
          version: targetFolder.version,
          input: { name, parentId: parentFolderId },
        },
        {
          onSuccess: close,
          onError: (reason: unknown) => setFolderError(folderOperationError(reason, "update")),
        },
      );
    else
      createFolder.mutate(
        { name, parentId: parentFolderId },
        {
          onSuccess: close,
          onError: (reason: unknown) => setFolderError(folderOperationError(reason, "create")),
        },
      );
  }

  function closeFolderDialog() {
    setFolderDialog(undefined);
    setFolderName("");
    setTargetFolder(undefined);
    setParentFolderId(null);
  }

  function restoreFolderOpener(event: { preventDefault: () => void }) {
    const opener = folderOpener.current;
    if (opener && document.contains(opener)) {
      event.preventDefault();
      opener.focus();
    } else if (createFolderButton.current && document.contains(createFolderButton.current)) {
      event.preventDefault();
      createFolderButton.current.focus();
    }
  }

  function deleteFolderAfterConfirm() {
    if (!deleteTarget || deleteFolder.isPending) return;
    const target = deleteTarget;
    setFolderError("");
    deleteFolder.mutate(
      { id: target.id, version: target.version },
      {
        onSuccess: () => {
          setDeleteTarget(undefined);
          setSearchError("");
          if (search.folderId === target.id)
            go({ folderId: target.parentId ?? undefined, page: 1 });
        },
        onError: (reason: unknown) => setFolderError(folderOperationError(reason, "delete")),
      },
    );
  }

  return (
    <section className="space-y-5" aria-labelledby="knowledge-list-title">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 id="knowledge-list-title" className="text-2xl font-bold">
            知識一覧
          </h1>
          <p className="mt-1 text-sm text-muted-foreground">保存した知識を検索・整理できます。</p>
        </div>
        <Button asChild>
          <Link to="/knowledge/new">知識を作成</Link>
        </Button>
      </div>
      <div className="grid gap-5 lg:grid-cols-[15rem_minmax(0,1fr)]">
        <Card>
          <CardHeader className="flex-row items-center justify-between gap-2">
            <CardTitle className="text-base">フォルダ</CardTitle>
            <Button
              ref={createFolderButton}
              size="sm"
              variant="outline"
              onClick={(event) => {
                folderOpener.current = event.currentTarget;
                setFolderError("");
                setFolderDialog("create");
                setFolderName("");
                setParentFolderId(search.folderId ?? null);
              }}
            >
              追加
            </Button>
          </CardHeader>
          <CardContent>
            {folders.isPending && <p role="status">読み込み中…</p>}
            {folders.isError && <p role="alert">フォルダを読み込めません。</p>}
            <nav aria-label="フォルダ一覧">
              <button
                type="button"
                className={`mb-1 w-full rounded-lg px-2 py-2 text-left text-sm hover:bg-accent ${!search.folderId ? "bg-secondary" : ""}`}
                onClick={() => {
                  setSearchError("");
                  go({ folderId: undefined });
                }}
              >
                すべての知識
              </button>
              <FolderTree
                folders={folders.data ?? []}
                selected={search.folderId}
                onSelect={(id) => {
                  setSearchError("");
                  go({ folderId: id });
                }}
                onRename={(folder, opener) => {
                  folderOpener.current = opener;
                  setFolderError("");
                  setTargetFolder(folder);
                  setFolderName(folder.name);
                  setParentFolderId(folder.parentId);
                  setFolderDialog("rename");
                }}
                onDelete={(folder, opener) => {
                  folderOpener.current = opener;
                  setFolderError("");
                  setDeleteTarget(folder);
                }}
              />
            </nav>
          </CardContent>
        </Card>
        <div className="min-w-0 space-y-4">
          <Card>
            <CardContent className="pt-6">
              <form
                className="flex flex-wrap gap-2"
                onSubmit={(event) => {
                  event.preventDefault();
                  const nextQuery = input.trim();
                  const error = validateSearchQuery(nextQuery);
                  if (error) {
                    setSearchError(error);
                    return;
                  }
                  setSearchError("");
                  go({ q: nextQuery });
                }}
              >
                <label htmlFor="knowledge-search" className="sr-only">
                  知識を検索
                </label>
                <Input
                  id="knowledge-search"
                  value={input}
                  onChange={(event) => setInput(event.target.value)}
                  aria-invalid={Boolean(searchError || queryError)}
                  aria-describedby={
                    searchError || queryError ? "knowledge-search-error" : undefined
                  }
                  placeholder="タイトル・タグ・本文を検索"
                />
                <Button type="submit">検索</Button>
                {(query || tags.length > 0) && (
                  <Button
                    type="button"
                    variant="ghost"
                    onClick={() => {
                      setInput("");
                      setSearchError("");
                      go({ q: undefined, tag: [] });
                    }}
                  >
                    条件をクリア
                  </Button>
                )}
              </form>
              {(searchError || queryError) && (
                <p
                  id="knowledge-search-error"
                  role="alert"
                  className="mt-2 text-sm text-destructive"
                >
                  {searchError || queryError}
                </p>
              )}
              {tags.length > 0 && (
                <p className="mt-3 text-sm text-muted-foreground">タグ: {tags.join(", ")}</p>
              )}
            </CardContent>
          </Card>
          {list.isPending && <p role="status">知識を読み込み中…</p>}
          {list.isError && (
            <Alert variant="destructive">
              <AlertDescription>{knowledgeError(list.error)}</AlertDescription>
              <Button
                variant="outline"
                onClick={() => {
                  list.refetch().catch(() => undefined);
                }}
              >
                再試行
              </Button>
            </Alert>
          )}
          {list.data?.items.length === 0 && (
            <Card>
              <CardContent className="pt-6">
                <p>条件に一致する知識はありません。</p>
              </CardContent>
            </Card>
          )}
          <div className="space-y-3" aria-live="polite">
            {list.data?.items.map((item) => (
              <KnowledgeRow
                key={item.id}
                item={item}
                folders={folders.data ?? []}
                search={{ q: query || undefined, tag: tags, folderId: search.folderId, pageSize }}
                onTagNavigate={() => setSearchError("")}
              />
            ))}
          </div>
          {list.data && list.data.page === page && list.data.pageSize === pageSize && (
            <nav aria-label="ページネーション" className="flex items-center justify-between gap-3">
              <span className="text-sm text-muted-foreground">
                {list.data.total === 0
                  ? "0件"
                  : `${list.data.total}件中 ${(page - 1) * pageSize + 1}〜${Math.min(page * pageSize, list.data.total)}件`}
              </span>
              <div className="flex items-center gap-2">
                <label className="flex items-center gap-2 text-sm">
                  <span>表示件数</span>
                  <select
                    aria-label="表示件数"
                    className="h-10 rounded-xl border border-input bg-card/85 px-3"
                    value={pageSize}
                    onChange={(event) => {
                      setSearchError("");
                      go({ page: 1, pageSize: Number(event.target.value) });
                    }}
                  >
                    {[10, 25, 50, 100].map((size) => (
                      <option key={size} value={size}>
                        {size}件
                      </option>
                    ))}
                  </select>
                </label>
                <Button
                  variant="outline"
                  disabled={page <= 1}
                  onClick={() => {
                    setSearchError("");
                    go({ page: page - 1 });
                  }}
                >
                  前へ
                </Button>
                <Button
                  variant="outline"
                  disabled={!list.data.hasNext}
                  onClick={() => {
                    setSearchError("");
                    go({ page: page + 1 });
                  }}
                >
                  次へ
                </Button>
              </div>
            </nav>
          )}
        </div>
      </div>
      {folderError && !folderDialog && (
        <p role="alert" className="text-sm text-destructive">
          {folderError}
        </p>
      )}
      <Dialog
        open={folderDialog !== undefined}
        onOpenChange={(open) => {
          if (!open && !createFolder.isPending && !updateFolder.isPending) closeFolderDialog();
        }}
      >
        <DialogContent onCloseAutoFocus={restoreFolderOpener}>
          <DialogHeader>
            <DialogTitle>
              {folderDialog === "rename" ? "フォルダ名を変更" : "フォルダを作成"}
            </DialogTitle>
            <DialogDescription>フォルダ名は1〜100文字で入力してください。</DialogDescription>
          </DialogHeader>
          {folderError && (
            <p role="alert" className="text-sm text-destructive">
              {folderError}
            </p>
          )}
          <form
            onSubmit={(event) => {
              event.preventDefault();
              saveFolder();
            }}
          >
            <Input
              aria-label="フォルダ名"
              value={folderName}
              onChange={(event) => setFolderName(event.target.value)}
            />
            <label className="mt-4 block space-y-2 text-sm">
              <span className="block font-medium">親フォルダ</span>
              <select
                aria-label="親フォルダ"
                className="h-11 w-full rounded-xl border border-input bg-card/85 px-3"
                value={parentFolderId ?? ""}
                onChange={(event) => setParentFolderId(event.target.value || null)}
              >
                <option value="">ルート</option>
                {(folders.data ?? [])
                  .filter((folder) => folder.id !== targetFolder?.id)
                  .map((folder) => (
                    <option key={folder.id} value={folder.id}>
                      {folder.name}
                    </option>
                  ))}
              </select>
            </label>
            <DialogFooter className="mt-4">
              <DialogClose asChild>
                <Button type="button" variant="outline">
                  キャンセル
                </Button>
              </DialogClose>
              <Button
                type="submit"
                disabled={!folderName.trim() || createFolder.isPending || updateFolder.isPending}
              >
                保存
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
      <AlertDialog
        open={deleteTarget !== undefined}
        onOpenChange={(open) => {
          if (!open && !deleteFolder.isPending) setDeleteTarget(undefined);
        }}
      >
        <AlertDialogContent onCloseAutoFocus={restoreFolderOpener}>
          <AlertDialogTitle>フォルダを削除しますか？</AlertDialogTitle>
          <AlertDialogDescription>中身があるフォルダは削除できません。</AlertDialogDescription>
          {folderError && (
            <p role="alert" className="text-sm text-destructive">
              {folderError}
            </p>
          )}
          <AlertDialogFooter>
            <AlertDialogCancel disabled={deleteFolder.isPending}>キャンセル</AlertDialogCancel>
            <AlertDialogAction
              disabled={deleteFolder.isPending}
              onClick={(event) => {
                event.preventDefault();
                deleteFolderAfterConfirm();
              }}
            >
              削除
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </section>
  );
}

const emptyTags: string[] = [];

function FolderTree({
  folders,
  selected,
  onSelect,
  onRename,
  onDelete,
  parentId = null,
  depth = 0,
}: {
  folders: Folder[];
  selected?: string;
  onSelect: (id: string) => void;
  onRename: (folder: Folder, opener: HTMLButtonElement) => void;
  onDelete: (folder: Folder, opener: HTMLButtonElement) => void;
  parentId?: string | null;
  depth?: number;
}) {
  return (
    <ul className="space-y-1" role={depth === 0 ? undefined : "group"}>
      {folders
        .filter((folder) => folder.parentId === parentId)
        .map((folder) => (
          <li key={folder.id}>
            <div className="flex items-center gap-1">
              <button
                type="button"
                className={`min-w-0 flex-1 truncate rounded-lg px-2 py-2 text-left text-sm hover:bg-accent ${selected === folder.id ? "bg-secondary" : ""}`}
                style={{ paddingLeft: `${8 + depth * 12}px` }}
                onClick={() => onSelect(folder.id)}
              >
                {folder.name}
              </button>
              <button
                type="button"
                className="rounded px-1 text-xs text-muted-foreground hover:bg-accent"
                aria-label={`${folder.name}を変更`}
                onClick={(event) => onRename(folder, event.currentTarget)}
              >
                変更
              </button>
              <button
                type="button"
                className="rounded px-1 text-xs text-muted-foreground hover:bg-accent"
                aria-label={`${folder.name}を削除`}
                onClick={(event) => onDelete(folder, event.currentTarget)}
              >
                削除
              </button>
            </div>
            <FolderTree
              folders={folders}
              selected={selected}
              onSelect={onSelect}
              onRename={onRename}
              onDelete={onDelete}
              parentId={folder.id}
              depth={depth + 1}
            />
          </li>
        ))}
    </ul>
  );
}

function KnowledgeRow({
  item,
  folders,
  search,
  onTagNavigate,
}: {
  item: KnowledgeListItem;
  folders: Folder[];
  search: KnowledgeListSearch;
  onTagNavigate: () => void;
}) {
  const move = useKnowledgeMove(item.id);
  return (
    <Card>
      <CardContent className="flex flex-wrap items-center gap-3 pt-5">
        <div className="min-w-0 flex-1">
          <Link
            to="/knowledge/$knowledgeId/edit"
            params={{ knowledgeId: item.id }}
            className="block truncate font-semibold hover:underline"
          >
            {item.title}
          </Link>
          <p className="mt-1 text-sm text-muted-foreground">
            {item.format === "html" ? "HTML" : "Markdown"} ·{" "}
            {new Date(item.updatedAt).toLocaleString("ja-JP")}
          </p>
          {item.tags.length > 0 && (
            <p className="mt-1 text-xs text-muted-foreground">
              {item.tags.map((tag) => {
                const tags = search.tag ?? emptyTags;

                const nextTags = tags.includes(tag)
                  ? tags.filter((selectedTag) => selectedTag !== tag)
                  : [...tags, tag];

                return (
                  <Link
                    key={tag}
                    to="/knowledge"
                    onClick={onTagNavigate}
                    search={{
                      q: search.q,
                      tag: nextTags.length ? nextTags : undefined,
                      folderId: search.folderId,
                      page: 1,
                      pageSize: search.pageSize,
                    }}
                    className="mr-2 hover:underline"
                  >
                    #{tag}
                  </Link>
                );
              })}
            </p>
          )}
        </div>
        <label className="flex items-center gap-2 text-sm">
          <span className="sr-only">{item.title}のフォルダ</span>
          <select
            aria-label={`${item.title}のフォルダ`}
            value={item.folder?.id ?? ""}
            disabled={move.isPending}
            onChange={(event) => {
              move.mutate({ version: item.version, folderId: event.target.value || null });
            }}
          >
            <option value="">ルート</option>
            {folders.map((folder) => (
              <option key={folder.id} value={folder.id}>
                {folder.name}
              </option>
            ))}
          </select>
          {move.isError && (
            <span role="alert" className="text-xs text-destructive">
              {folderOperationError(move.error, "move")}
            </span>
          )}
        </label>
      </CardContent>
    </Card>
  );
}
