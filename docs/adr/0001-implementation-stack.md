# ADR-0001: Knowledge Base の実装技術スタック

- 状態: 採用
- 決定日: 2026-09-06
- 関連: GitHub Issue #2

## 決定

| 領域 | 採用技術 | バージョン方針 |
| --- | --- | --- |
| クライアント | React + TypeScript + Vite、TanStack Router、TanStack Query、Tailwind CSS、shadcn/ui | 下記の採用時点の最新安定版を完全固定する。更新は依存更新PRで明示的に行い、`pnpm-lock.yaml` をコミットする。shadcn/uiで取り込むコンポーネントのソースはリポジトリ管理する。 |
| Markdownプレビュー | `react-markdown` + `remark-gfm` | Markdownの表示変換はフロントエンド責務とする。Markdownに含まれる生HTMLは既定で描画しない。ライブラリは`package.json`とlockfileで完全固定する。 |
| サーバー | Go + Gin + `pgx/v5` / `pgxpool` | Ginと`pgx/v5`は下記の採用時点の最新安定版を完全固定する。PostgreSQL専用の手書きSQLを使い、ORM・SQL生成は採用しない。Goはサポートされる2世代内を維持し、依存は `go.mod` / `go.sum` に固定する。 |
| RDBMS | PostgreSQL | 採用時点の最新安定版を完全固定し、開発・CI・本番で同じイメージダイジェストを使用する。パッチ更新は更新PRで明示的に行う。 |
| migration | `golang-migrate/migrate` v4、SQL migration | `up` / `down` を対にしてリポジトリ管理し、適用済みファイルは変更しない。CLI はコンテナイメージのダイジェストで固定する。 |
| ファイル | 開発・CI: MinIO、本番: Amazon S3 | 開発・CIではMinIOをDocker Composeのサービスとして稼働させる。本番ではS3 private bucketへ保存する。SDKはGo moduleで固定する。上限10 MiBをHTTP受信前・ストリーム中・保存前に検査する。 |
| HTML隔離 | サーバー側 `bluemonday` + 専用オリジンの sandboxed `iframe` + CSP | 許可リストはテストで固定する。`srcdoc` は使わず、隔離オリジンから静的HTMLを配信する。 |
| GitHub OAuth | GitHub OAuth App + `golang.org/x/oauth2` | OAuth Appは本番・非本番で分離し、ライブラリはGo moduleで固定する。 |
| 認証・MCP | JWT access token、ローテーションRefresh Token、ランダムなMCP個人トークン | access token は短命、Refresh Token はハッシュ化してDB保持・再利用時はトークンファミリーを失効。MCPトークンも平文は発行応答の一度だけ表示し、DBには検証用ハッシュだけを保存する。 |
| デプロイ・CI | Docker Compose、Cloudflare、GitHub Actions、pnpm、Go toolchain | アプリケーション、PostgreSQL、reverse proxyはDocker Composeで構成する。CloudflareはDNS、TLS終端、リバースプロキシ、WAFに使用する。third-party ActionはフルコミットSHAで固定する。Node / Go / PostgreSQLイメージもメジャーだけにせず、更新PRで明示的に上げる。 |
| E2E | Playwright | ブラウザを必要とする認証、登録、更新、公開閲覧、MCPトークン発行・失効、HTMLプレビュー隔離をPlaywrightで検証する。ブラウザとnpm依存はlockfileで固定する。 |
| Go単体テスト | 標準 `testing` + `testify/assert` / `testify/require` | Domain・Use case・HTTP handlerは外部依存をinterface越しの手書きfakeへ差し替えて単体テストする。`testify/mock`は原則採用せず、PostgreSQL・MinIO・OAuth連携は結合テストで検証する。 |

## 構成と責務

```text
React SPA (Vite) ── HTTPS / JSON ──> Go API (Gin)
        │                                      │
        │ public HTML: separate origin          ├─ PostgreSQL
        └──────── sandboxed iframe ────────────┼─ MinIO（開発・CI）/ S3（本番）
                                               └─ GitHub OAuth
```

- SPAはTanStack Routerで画面遷移を、TanStack Queryでサーバー状態を管理する。shadcn/uiは採用したコンポーネントのソースをリポジトリ内で管理し、画面・入力・API呼出しを担う。認可判断、秘密値、HTML安全化は担わない。
- Go API（Gin）はCookie、認可、OAuth callback、入力検証、業務トランザクション、監査、ファイル上限検査を担う。
- PostgreSQLはユーザー、知識、公開状態、Refresh Tokenのハッシュと失効状態、MCPトークンのハッシュ、監査記録を永続化する。ファイル、Markdownファイル、HTMLファイルは格納しない。開発・CIではMinIOの永続ボリューム、本番ではS3 private bucketへ保存する。
- Markdownはフロントエンドでプレビューする。プレビュー対象のHTMLはデプロイ済みアプリのHTMLではなく、ユーザーが登録したHTMLファイルである。HTMLファイルは保存時に許可リストで安全化し、閲覧時もアプリと異なるプレビュー用サブドメインで表示する。埋め込み側は `src` を使い、`<iframe sandbox referrerpolicy="no-referrer">` に `allow-*` トークンおよび `allow` 属性を付けない。`srcdoc` は使わない。
- プレビュー応答は `default-src 'none'`、`script-src 'none'`、`connect-src 'none'`、`object-src 'none'`、`base-uri 'none'`、`form-action 'none'` を含むCSPを返す。`frame-ancestors` はアプリの正確なoriginだけを許可し、`X-Content-Type-Options: nosniff` も返す。アプリの認証Cookieは`Domain`属性なしのhost-only Cookie（可能なら`__Host-`接頭辞）にして、プレビュー用サブドメインへ送らない。

## 要件への適合根拠

| 要件 | 採用による担保 |
| --- | --- |
| JWT / Refresh Token | `HttpOnly; Secure; SameSite=Lax` Cookieで送信し、access tokenは短命にする。Refresh Tokenはローテーション、ハッシュ保存、再利用検知時のファミリー全失効をサーバーで実施する。クライアントのlocalStorage等には保持しない。 |
| GitHub OAuth | Authorization Code flowをサーバーで完結し、PKCEと検証済みの`state`を必須にする。client secretはサーバー環境変数／Secretにのみ置く。 |
| 10MBファイル | `Content-Length`だけを信用せず、最大10 MiBのリーダーで読み込み、MIME・拡張子・所有者を検証してからオブジェクトストレージへ保存する。ダウンロードは認可後の短期署名URLまたはAPIストリームに限る。 |
| 公開表示 | 公開取得APIは公開専用DTOを返し、所有者、メール、トークン、監査、非公開下書き、内部ストレージキーを投影しない。公開HTMLも隔離オリジンで表示する。 |
| MCPトークン | 256 bit以上のCSPRNG値を接頭辞付きで発行し、平文は作成直後の一度だけ返す。表示後・ログ・監査・エラー・分析基盤に平文を残さず、ハッシュと失効日時だけを保存する。 |

## ローカル開発とCI

実装開始時に、以下のファイルとコマンドを置く。コマンド名は以後の規約・CIで共通に用いる。

| 目的 | 設定ファイル | ローカル / CI コマンド |
| --- | --- | --- |
| クライアント依存・ビルド | `client/package.json`、`client/pnpm-lock.yaml`、`client/vite.config.ts` | `pnpm --dir client install --frozen-lockfile`、`pnpm --dir client lint`、`pnpm --dir client test`、`pnpm --dir client build` |
| サーバー依存・検査 | `dev/backend/go.mod`、`dev/backend/go.sum`、`dev/backend/.golangci.yml` | `go -C dev/backend test ./...`、`golangci-lint run ./dev/backend/...`、`go -C dev/backend vet ./...` |
| DB・migration | `dev/backend/migrations/*.sql`、`compose.yaml` | `docker compose up -d postgres minio`、`migrate -path dev/backend/migrations -database "$DATABASE_URL" up`、`migrate -path dev/backend/migrations -database "$DATABASE_URL" down 1` |
| デプロイ・横断試験 | `compose.yaml`、`dev/compose.test.yaml`、各サービスの`Dockerfile`、`dev/frontend/e2e/`、ルート`Taskfile.yml` | `docker compose up --build -d`。横断試験は実行固有の`E2E_RUN_ID`を指定し、`task e2e:env:up`、`task e2e:env:init`、`task e2e:test`、`task e2e:env:down`を順に実行する。準備と実装状況は[横断検証規約](../testing.md)を参照する。 |
| CI | `.github/workflows/ci.yml` | push と pull request で、依存固定確認、lint、unit test、migration適用、Playwright E2E、Docker Compose buildを実行する。 |

CIはGitHub OAuthとCloudflareの実サービスを呼ばない。テスト用OAuthプロバイダをGo APIのポートで差し替え、DBとMinIOはDocker Composeの使い捨てコンテナを使用する。Cloudflareの設定は構成ファイルとテストで検証し、ブラウザE2Eはローカルのプレビュー用ホスト名を使用する。

## 採用時点の固定バージョン

2026-09-06に確認した最新安定版を初期値とする。`latest`タグやバージョン範囲指定をデプロイに使わず、`compose.yaml`ではコンテナイメージのダイジェスト、`package.json`と`go.mod`では下記の完全バージョンを固定する。

| 技術 | 初期固定バージョン |
| --- | --- |
| Node.js | 24.20.0 LTS |
| React | 19.2.8 |
| Vite | 8.2.2 |
| TypeScript | 7.0.2 |
| TanStack Router | 1.170.32 |
| TanStack Query | 5.102.8 |
| shadcn CLI | 4.21.0 |
| react-markdown | 10.1.0 |
| remark-gfm | 4.0.1 |
| Go | 1.27.1 |
| Gin | 1.12.0 |
| pgx | 5.10.0 |
| PostgreSQL | 18.1 |
| Tailwind CSS / Playwright / MinIO | `package.json` または `compose.yaml` の作成時に最新安定版を確認し、lockfileまたはイメージダイジェストで完全固定する。タグ`latest`は使用しない。 |

shadcn/uiはコンポーネントを生成してアプリ側で管理する方式であるため、UI本体に一律のランタイムバージョンはない。CLIのみを上記の初期値で固定し、取り込んだコンポーネントは依存更新PRで個別に更新する。

## 更新規則

- 依存更新はlockfile、`go.mod` / `go.sum`、コンテナイメージの更新を同一PRに含める。
- メジャー更新、HTML許可リストの緩和、認証Cookie属性の変更、PostgreSQLメジャー更新は新規ADRを作成してから行う。
- 具体的なディレクトリ構造、テストの配置、画面実装の分割は本ADRの範囲外であり、Issue #3〜#7で決定する。

## 参考

- [React versioning](https://react.dev/versions)
- [Vite releases](https://vite.dev/releases)
- [Go release policy](https://go.dev/doc/devel/release)
- [Gin releases](https://github.com/gin-gonic/gin/releases)
- [Playwright](https://playwright.dev/)
- [GitHub Actions CI](https://docs.github.com/en/actions/get-started/continuous-integration)
