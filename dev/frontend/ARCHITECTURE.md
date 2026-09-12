# フロントエンド・アーキテクチャー

## 目的と適用範囲

この文書は `dev/frontend` にある React SPA の、ディレクトリとファイルの責務境界を定める。採用技術は React、Vite、TanStack Router、TanStack Query、Tailwind CSS、shadcn/ui、`react-markdown` である。

新しい画面・機能・共通部品を追加するときは、この文書の配置と依存方向に従う。サーバー、DB、API仕様そのものの責務は対象外とする。

## 全体構成

```text
dev/frontend/
  index.html                 # Vite のHTMLエントリ
  vite.config.ts             # Vite、Router、Tailwind、Vitestの設定
  tailwind.config.cjs        # 参照UIと共通の色・影・角丸トークン
  components.json            # shadcn/ui CLI の生成設定
  src/
    main.tsx                 # Reactの起動点
    router.tsx               # Router、QueryClient、Providerの生成
    router-context.ts        # ルートが参照する共有コンテキストの型
    routeTree.gen.ts         # Routerプラグインの生成物。編集禁止
    routes/                  # URL、認可境界、URL状態、loader
    features/                # 業務機能
    components/              # 機能横断の表示部品
    lib/                     # 機能横断の技術的な共通処理
    styles/                  # 全体スタイルとテーマ
```

依存方向は次の一方向を原則とする。

```text
routes → features → lib / components/ui
components（共通）→ lib / components/ui
```

`lib`、`components/ui`、別機能の `features` が `routes` を参照してはならない。機能間で型や処理を直接再利用せず、本当に共通化すべき責務だけを `lib` または `components` へ昇格させる。

### リントで検査する境界

`oxlint.config.ts` は `src` のディレクトリを実行時に調べ、新しく追加した機能にも次の制約を適用する。

- 異なる `features/<feature>` 間の参照は、`import type` と再エクスポートも含めて禁止する。
- 共通の `lib`・`components` から `features`・`routes` への参照、および機能から `routes` への参照を禁止する。
- 機能の `ui` から、自機能の `api`・共通の `lib/http`・`axios` への参照を禁止する。単純な通信でも `ui → model → api` を経由する。
- 機能の `ui` では `fetch`・`XMLHttpRequest` の直接使用も禁止する。`window.fetch` なども対象にする。
- 同じ機能の `api` から `model` の型を参照することは許可する。
- `import/no-cycle` と `import/no-self-import` で循環依存と自己importを禁止する。循環依存は公式の標準設定を使用し、型のみのimportは循環の検査から除外する。ただし、異なる機能間の型参照は禁止のままである。テストファイルにも適用する。

importの検査は `@/` と正規化した相対パスを対象とする。静的import、再エクスポート、文字列リテラルの動的importを検査する。文字列を組み立てる動的import、別名の追加、冗長な `./`・`../` を挟むパスまで解決する仕組みではないため、これらで境界を迂回してはならない。新しいパス別名や通信ライブラリを導入する際は設定も更新する。

### 状態分岐の網羅性

Union型・enumの `switch` はすべての状態を明示的な `case` で処理する。`default` を書いても状態の省略は許可しない。`typescript/switch-exhaustiveness-check` で検査する。`if`・三項演算子やAPI応答の実行時検証は、このルールの対象外である。

## `src/routes/` — URLと遷移の境界

TanStack Router のファイルベースルーティングだけを置く。各ルートは以下だけを担当する。

- URLパス、path parameter、検索パラメータの検証
- 認可境界とリダイレクト（`beforeLoad`）
- 画面到達前に必須の軽量なloader
- 遅延読込する画面コンポーネントの指定

画面UI、HTTP実装、フォーム状態、業務判断は置かない。画面は対応する `features/*/ui/pages/` から読み込む。

ルート設定は `*.tsx`、遅延読込される画面・pending・error表示は `*.lazy.tsx` に分ける。TanStack Routerの自動コード分割を維持するためである。

`routeTree.gen.ts` は Vite のRouterプラグインが生成する。手編集、手動作成、レビュー対象の変更は禁止する。ルートを追加・移動した場合は `pnpm build` または `pnpm dev` で更新する。

## `src/features/<feature>/` — 機能の境界

知識、認証、公開知識、MCPトークンのように、利用者に見える業務能力ごとに作る。画面単位で切りすぎず、同じデータ・操作・権限を扱う画面群は一つの機能にまとめる。

```text
features/<feature>/
  api/       # HTTP契約
  model/     # サーバー状態、入力モデル、検証
  ui/        # ページと表示部品
  lib/       # その機能だけで使う純粋処理（必要な場合のみ）
```

### `api/` — HTTP契約

- API呼出し、request/response DTO、DTOから機能モデルへの変換入口を置く。
- Axiosの共通設定はここに重複させず、`lib/http/client.ts` を使う。
- TanStack Query、React Hook、トースト、画面遷移、DOMを参照しない。
- APIエンドポイントが未確定なら、推測で本実装を置かず、型・境界だけを先に置く。

### `model/` — データと操作の状態

- TanStack Query のquery key、query options、mutationを置く。
- mutation成功時の関連query無効化、楽観更新、入力検証、機能固有の型を置く。
- query key は機能名を先頭にしたシリアライズ可能な配列にし、IDや検索条件を必ず含める。
- URL共有が必要な検索・ページング・絞り込み状態はここではなく `routes/` の検索パラメータとして扱う。
- Queryキャッシュへ秘密情報や一度限りの平文トークンを保存しない。

### `ui/` — React表示

```text
ui/
  pages/       # routeから直接読み込まれる画面の合成
  components/  # 当該機能だけで使う表示部品
  hooks/       # 表示・イベントに密着するHook（必要な場合のみ）
```

- `pages/` は画面を組み立て、`model/` のquery・mutationを呼ぶ。
- `components/` はPropsで表現できる表示とユーザー操作を担う。HTTPの詳細を直接持たない。
- `hooks/` は未保存変更確認、キーボード操作などUIに限定する。データ取得Hookは `model/` に置く。

### `lib/` — 機能内の純粋処理

- React、HTTP、TanStack Queryに依存しない整形、比較、変換を置く。
- 他機能でも同じ責務が必要になった時だけ `src/lib/` へ移す。

## `src/components/` — 機能横断の表示部品

`app-shell.tsx`、読み込み表示、共通エラー表示など、複数機能が同じ振る舞いで使う部品を置く。

- `components/ui/` はshadcn/ui系の基礎部品専用である。参照元の `topic2html` と同じ実装を管理する。
- 業務語彙を持つ `KnowledgeForm`、`McpTokenList` などは置かず、対応する `features/*/ui/components/` に置く。
- `components/ui/` の変更は、汎用部品としてのアクセシビリティ・API互換性を保つ。
- Storybook用の `*.stories.tsx` は、このプロジェクトでStorybookを導入するまで追加しない。

## `src/lib/` — 横断技術の共通処理

- `lib/http/`: Cookie送信、JSON応答、共通APIエラー。個別エンドポイントは置かない。
- `lib/markdown/`: Markdown表示の共通設定。`MarkdownRenderer` は `remark-gfm` を使用し、生HTMLは描画しない。
- `lib/utils.ts`: `cn` のような副作用のない汎用関数。

認可判断、OAuthの秘密情報、HTML安全化、トークン平文の永続化をクライアントへ実装してはならない。これらはサーバーの責務である。

## `src/styles/` とテーマ

`styles/globals.css` はTailwindの読み込み、CSS変数、全体のbase styleだけを置く。テーマトークンは `tailwind.config.cjs` に対応する名称を使う。

- コンポーネント固有の見た目はTailwindのユーティリティクラスで表現する。
- 機能専用の巨大なグローバルCSSは追加しない。
- `components/ui/` を追加・変更するときは、既存トークン（`primary`、`card`、`muted`、`destructive` 等）を優先する。

## テストの配置と実行

コードの空行は `@stylistic/padding-line-between-statements` で検査する。関数宣言と複数行の変数宣言の前後には空行を入れる。exportされた宣言も対象にし、短い変数宣言が連続する場合には空行を強制しない。

空行の自動修正は `pnpm lint:fix`、その他の整形は `pnpm fmt` で行う。整形で宣言が複数行に変わった場合は、再度 `pnpm lint:fix` を実行する。CIでは `pnpm lint` と `pnpm fmt:check` で両方を確認する。

テストは対象ファイルの隣に `*.test.ts` または `*.test.tsx` として置く。

```text
lib/markdown/markdown-renderer.tsx
lib/markdown/markdown-renderer.test.tsx
features/knowledge/model/knowledge-schema.ts
features/knowledge/model/knowledge-schema.test.ts
```

- UI部品はVitestとTesting Libraryで、表示・操作・アクセシビリティを検証する。
- `model/` はquery key、mutation後の無効化、入力検証を検証する。
- `lib/markdown/` は生HTMLを描画しないことを回帰テストする。
- API結合やOAuthを含むブラウザ横断の確認は、将来の `e2e/` でPlaywrightが担う。

実装変更後は、次をすべて実行する。

```bash
pnpm --dir dev/frontend build
pnpm --dir dev/frontend lint
pnpm --dir dev/frontend test
```

## 例外と共通化の判断

次のすべてを満たす場合だけ、機能内のコードを `components/` または `lib/` へ移す。

1. 二つ以上の機能が同じ責務を必要としている。
2. API、業務語彙、権限条件に依存しない。
3. Propsまたは引数だけで再利用できる。
4. 共通化後も呼び出し側の意図が明確である。

見た目が似ているだけ、または将来使うかもしれないだけでは共通化しない。
