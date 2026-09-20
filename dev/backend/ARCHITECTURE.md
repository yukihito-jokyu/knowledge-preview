# バックエンドのアーキテクチャ

この文書は、Goバックエンドの各パッケージの責務と、実装を追加する場所を定める。
GinによるHTTP API、pgxによるPostgreSQL接続、ヘルスチェック、OAuth認証、知識の保存・隔離配信を実装している。
認証の契約・共有schema・設定・検証は [KNOWLEDGE.md](KNOWLEDGE.md) を参照する。
開発コマンドは [Taskfile.yml](Taskfile.yml)、Lint・整形の設定は [.golangci.yml](.golangci.yml)、
golangci-lintの指定バージョンは [.golangci-lint-version](.golangci-lint-version) を参照する。

## パッケージの責務

HTTPの受付、アプリケーションの処理、外部技術へのアクセスを分ける。
これらの実装を生成して接続する場所は `cmd/` 配下の各実行プログラムとする。

| 配置 | 責務 | 現在の実装 |
| --- | --- | --- |
| `cmd/api` | 設定読込み、依存の組立て、サーバーの起動・終了 | [main.go](cmd/api/main.go) |
| `cmd/e2etest` | E2E専用サーバーの起動、テスト用認証・データ準備、依存の組立て | [main.go](cmd/e2etest/main.go) |
| `internal/config` | 環境変数の読込みと設定の検証 | [config.go](internal/config/config.go) |
| `internal/domain` | 業務上の概念・ルール・エラー | [error.go](internal/domain/error.go) |
| `internal/usecase` | 処理の流れと、処理に必要な外部依存の抽象 | [health.go](internal/usecase/health.go) |
| `internal/interface/http` | ルーティング、リクエストの受付、middleware | [router.go](internal/interface/http/router.go)、[health.go](internal/interface/http/health.go) |
| `internal/interface/response` | レスポンスの型、エラーからHTTP応答への変換と出力 | [error.go](internal/interface/response/error.go) |
| `internal/infrastructure/postgres` | PostgreSQL接続など、pgxを使う実装 | [pool.go](internal/infrastructure/postgres/pool.go) |
| `internal/infrastructure/github` | GitHub OAuthの認可コード交換とユーザー取得 | [oauth.go](internal/infrastructure/github/oauth.go) |
| `migrations` | DBスキーマ変更のSQLを置く場所 | [知識schema](migrations/000001_knowledge.up.sql)、[知識一覧・folder拡張](migrations/000003_knowledge_library.up.sql)。適用はgolang-migrateで実施 |

## ファイルとディレクトリの分け方

機能ごとのファイルを各パッケージの直下に置き、分類だけを目的に階層を増やさない。
現在のヘルスチェックは `usecase/health.go` と `interface/http/health.go` に配置している。
パッケージ名から役割が分かる場合、ファイル名に `_handler` などの役割名を重ねない。

- Use caseは `internal/usecase/<機能名>.go` に置く。
- HTTPハンドラーは `internal/interface/http/<機能名>.go` に置く。
- レスポンスの型と出力処理は `internal/interface/response` にまとめる。共通エラーは `error.go` に置く。
- 外部技術の実装は `internal/infrastructure/postgres` のように技術名を直下に置く。`persistence` の中間階層は設けない。
- テストは対象の実装と同じディレクトリに `*_test.go` として置く。

新しいサブパッケージは、独立した責務や公開範囲を分ける必要が生じたときに検討する。

## 依存の向きと組立て

HTTPハンドラーはUse caseを呼び出し、必要なレスポンスを `response` パッケージで出力する。
Use caseには `gin.Context` やpgxの具体型を渡さず、`context.Context` と必要な操作を表すinterfaceを使う。
業務ルールを扱う `domain` はGin、pgx、環境変数の読込みへ依存しない。
`response` はドメインエラーをHTTPへ変換するため、`domain` とGinを利用する。

例えば [usecase/health.go](internal/usecase/health.go) は、接続プールではなく次のinterfaceを要求する。

```go
type ReadinessChecker interface {
	Ping(context.Context) error
}
```

[cmd/api/main.go](cmd/api/main.go) でPostgreSQLの接続プールを渡す。
`*pgxpool.Pool` が `Ping` を持つため、この段階では専用のラッパーを追加していない。

```go
readiness := usecase.NewReadinessUseCase(pool)
router := httpinterface.NewRouter(readiness, logger)
```

テストでは同じinterfaceを満たすfakeを渡せるため、HTTPの検証に実DBを必要としない。

## 起動とリクエストの処理

起動時は `config.Load` で設定を読み込み、`postgres.NewPool` で接続プールを作成して疎通を確認する。
その後、Use caseとルーターを組み立ててHTTPサーバーを起動する。
設定読込みやDB接続が失敗した場合は、ログを出力して終了する。
終了シグナルを受けた場合は、10秒の期限でHTTPサーバーの終了処理を行う。

| エンドポイント | 処理 | 成功時 | 失敗時 |
| --- | --- | --- | --- |
| `GET /health` | プロセスの応答を確認。DBへアクセスしない | 204 | 専用の失敗分岐なし |
| `GET /ready` | 2秒の期限でUse caseからDBの `Ping` を呼ぶ | 204 | 共通エラーレスポンス。通常の接続エラーは500 |

ルーターは `gin.New()` で作成し、リクエストログ、`gin.Recovery()`、認証のprocess-wide rate limitを登録する。
Ginは転送ヘッダーを信用しない。認証の利用者別制限は境界プロキシで実施し、境界プロキシは`X-Forwarded-For`と`X-Real-IP`を接続元IPで上書きして、OAuthのstart/callbackを同じIP単位で10回/分に制限する。proxyを経由しない直結経路には、同じstart/callbackを共有するcapacity 600・補充600回/分のprocess-wide bucketを安全網として適用する。後者は複数利用者で共有する上限であり、利用者別10回/分の代替ではない。
[middleware.go](internal/interface/http/middleware.go) は `X-Request-ID` を引き継ぎ、未指定なら生成する。
リクエストIDは応答ヘッダーと、メソッド・パス・ステータス・処理時間を含む構造化ログに記録する。

## エラーレスポンス

ハンドラーはエラーを `response.WriteError` に渡す。
共通の応答形式は [response/error.go](internal/interface/response/error.go) で定義する。

```json
{"error":{"code":"internal","message":"an unexpected error occurred"}}
```

| エラー | HTTPステータス | `code` |
| --- | --- | --- |
| `domain.ErrNotFound` | 404 | `not_found` |
| `domain.ErrForbidden` | 403 | `forbidden` |
| `domain.ErrConflict` | 409 | `conflict` |
| `domain.ErrValidation` | 422 | `validation_failed` |
| その他 | 500 | `internal` |

判定には `errors.Is` を使うため、ラップされたドメインエラーも変換できる。
内部エラーの詳細は応答へ含めない。現在の `WriteError` 自体にはエラー詳細のログ出力はなく、
リクエストログにもエラー原因は含まれない。
また、`gin.Recovery()` によるpanic応答は、この共通JSON変換を経由しない。

## 検証範囲

実フロントエンドとのAPI契約、認証Cookie、ブラウザE2E、公開閲覧、回帰の該当条件と完了条件は [横断検証規約](../../docs/testing.md) に従う。各層のテストは本ディレクトリに、横断テストは`dev/frontend/e2e/` に配置する。

[config_test.go](internal/config/config_test.go) は必須設定とデフォルト値を検証する。
[router_test.go](internal/interface/http/router_test.go) はfakeを使い、ヘルスチェック成功時の204とリクエストIDを検証する。
実DB接続、readiness失敗時、各ドメインエラーのHTTP変換を個別に検証するテストは、現在は未実装である。

全体の検証はリポジトリルートから `task --dir dev/backend check` で実行する。
構成を変更した際は、この文書の配置例と実装へのリンクも更新する。

## 知識の追加配置

`domain/knowledge.go`・`domain/library.go`・`domain/markdown.go` が型・本文・検索条件検証、`usecase/knowledge.go` がupload/list/folder処理と外部interface、`interface/http/knowledge.go` が受付、`interface/response/knowledge.go` が所有者DTO、`infrastructure/postgres/knowledge.go` がSQL transactionを担う。独立した外部技術として `infrastructure/htmlsafe` がHTML安全化、`infrastructure/s3` がMinIO/S3を扱う。`cmd/collect-knowledge` は未参照object回収CLIである。Use caseへGin/pgx/S3具体型は渡さない。

## Goコードの改行

`task --dir dev/backend fmt` は `golines` で長い行を120文字を目安に改行する。テーブル駆動テストの構造体ケースは、短い場合も各フィールドを別行に書く。既存の複数行表記は整形後も維持する。文字列リテラルの内部は自動分割しない。`task --dir dev/backend check` で整形違反も検査する。
