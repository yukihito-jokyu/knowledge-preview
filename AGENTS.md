# 実装に取りかかるとき

人間に渡す説明は前提と理由をつなげて簡潔に書く。根拠を示す際は実際の記述を短く引用し、ファイルの絶対パスと行番号へのリンクを添える。

まず対象Issueの要件・受け入れ基準、`git status --short`と既存差分を確認する。既存の変更を上書きしない。次の順に参照し、仕様・規約・現在の実装を区別する。

1. [README](README.md)から設計文書へ進む。
2. [技術選定ADR](docs/adr/0001-implementation-stack.md)で認証、公開情報、HTML隔離、保存先の要件を確認する。
3. 対象層のARCHITECTURE.mdと、変更するコード・呼出し元・近いテストを読む。
4. [横断検証規約](docs/testing.md)でAPI契約・Cookie・E2E・回帰の該当条件を確認する。共通シナリオは最低限の観点であり、各Issueで具体的な初期状態、入力、操作、期待結果、失敗・境界条件を決める。

## フロントエンドの実装

実装時は[frontend-implementスキル](.agents/skills/frontend-implement/SKILL.md)と[フロントエンド構成](dev/frontend/ARCHITECTURE.md)に従う。

| 確認する内容 | 参照先 |
| --- | --- |
| 依存・検証コマンド | [package.json](dev/frontend/package.json)、[lockfile](dev/frontend/pnpm-lock.yaml) |
| 型・Lint・整形 | [tsconfig.app.json](dev/frontend/tsconfig.app.json)、[oxlint.config.ts](dev/frontend/oxlint.config.ts)、[Oxfmt設定](dev/frontend/.oxfmtrc.json) |
| URL・認可・画面接続 | `dev/frontend/src/routes/`、[router.tsx](dev/frontend/src/router.tsx) |
| 画面からHTTPまでの責務 | `dev/frontend/src/features/<feature>/ui/` → `model/` → `api/` |
| Cookie送信・エラー変換 | [HTTPクライアント](dev/frontend/src/lib/http/client.ts)、バックエンドの対応するHTTP・response実装 |
| 共有部品とテーマ | `dev/frontend/src/components/ui/`、[globals.css](dev/frontend/src/styles/globals.css) |
| テスト設定・CI | [vite.config.ts](dev/frontend/vite.config.ts)、[Frontend checks](.github/workflows/frontend-checks.yml) |

UIから直接HTTPを呼ばず、別機能を直接importしない。`routeTree.gen.ts`は手編集せずbuildで生成する。未確定のAPIを推測で実装しない。単体テストは対象コードの隣に置き、検証コマンドはスキルとpackage.jsonを参照する。

依存は`dev/frontend/package.json`とlockfileで一元管理する。`task e2e:install`はフロントエンド全体の依存を導入する。E2Eのブラウザ実行はVitestから分離する。

## バックエンドの実装

実装時は[backend-implementスキル](.agents/skills/backend-implement/SKILL.md)と[バックエンド構成](dev/backend/ARCHITECTURE.md)に従う。

| 確認する内容 | 参照先 |
| --- | --- |
| ツール・依存・検証 | [Taskfile.yml](dev/backend/Taskfile.yml)、[go.mod](dev/backend/go.mod)、[Lint設定](dev/backend/.golangci.yml)、[Lintバージョン](dev/backend/.golangci-lint-version) |
| 起動と依存の組立て | [cmd/api/main.go](dev/backend/cmd/api/main.go) |
| 設定とDB接続 | [config.go](dev/backend/internal/config/config.go)、[.env.example](dev/backend/.env.example)、[pool.go](dev/backend/internal/infrastructure/postgres/pool.go) |
| HTTP受付・レスポンス | [router.go](dev/backend/internal/interface/http/router.go)、`internal/interface/http/`、[response/error.go](dev/backend/internal/interface/response/error.go) |
| 業務処理・ルール | `dev/backend/internal/usecase/`、`dev/backend/internal/domain/` |
| 既存テスト・CI | [router_test.go](dev/backend/internal/interface/http/router_test.go)、[Backend quality](.github/workflows/backend-quality.yml) |

HTTP → Use case → 外部依存のinterfaceという責務を守り、Use caseへGinやpgxの具体型を持ち込まない。API変更時はフロントエンドの対応するapi・modelも確認する。Goテストは対象実装と同じディレクトリに置く。検証は`task --dir dev/backend check`、整形は`task --dir dev/backend fmt`を使う。

## 両層を接続する変更

[横断検証規約](docs/testing.md)で必須シナリオを選び、各Issueで詳細を具体化する。[E2E README](dev/frontend/e2e/README.md)から[Compose](dev/compose.test.yaml)、[ルートTaskfile](Taskfile.yml)へ進む。

環境構築は`task e2e:env:up`、初期化は`task e2e:env:init`、テスト開始は`task e2e:test`、後片付けは`task e2e:env:down`。同じ実行固有のE2E_RUN_IDを指定する。一括実行は`task e2e:check`を使う。

現在は既存SPA・API・DBの環境疎通まで実装済みであり、業務処理の未実装箇所はE2E READMEに記載している。雛形の配置・設定検証と実E2E成功を混同しない。規約やコマンドを変更した場合は参照文書も更新し、検証結果と未確認事項を報告する。

E2Eの作成・修正は[e2e-implementスキル](.agents/skills/e2e-implement/SKILL.md)、レビューは[e2e-reviewスキル](.agents/skills/e2e-review/SKILL.md)に従う。配置と増やし方は[横断検証規約のディレクトリ設計](docs/testing.md#e2eのディレクトリ設計)を参照する。
