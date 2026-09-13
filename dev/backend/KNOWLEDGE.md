# 知識の保存・配信（Issue #14）

`internal/usecase/knowledge.go` が詳細、最近10件、本文保存、draft確定、公開切替、HTML配信を実装する。HTTPは `/api/v1/knowledge` と `/api/v1/knowledge-drafts` に登録する。認証 #19 は未実装のため、標準起動の管理APIは401で拒否する。テストだけの認証や所有者指定ヘッダーを製品へ追加していない。

## 接続する境界

- #19: `http.Authenticate` で署名検証済み `domain.Principal{OwnerID, SessionID}` を返し、`usecase.Sessions.Active` をプライマリDBのsession有効性照会へ実装する。標準起動の `nil` と `DenySessions{}` を同時に置き換える。認証Cookieはhost-onlyとし、logoutは失効commit後に成功を返す。照会は発行時・各HTML配信時に行い、キャッシュやJWTのみの判定は禁止する。URLを持つ人は発行元sessionが有効な間は本文を取得できる。配信済み本文の遠隔消去は行わない。
- #15: アップロード、初期metadata抽出、フォルダ作成は未実装。`knowledge_folders` と `knowledge_drafts` の共有schemaを使い、private objectを `knowledge_objects` へ記録してdraftから参照する。sourceやmetadataを時間で消さない。HTML検証は `domain.ApplySource` と `htmlsafe.New().Sanitize` を共有する。draftから参照中のobjectと確定済みIDは回収しない。
- #16: 検索・一覧は未実装。knowledgeのmetadataとsource参照を使用する。閲覧のみでは `updated_at` を変えない。
- #17: 公開DTO/画面は未実装。`KnowledgeRepository.Public` は publicId・現在version・unlistedを毎回照合する。公開DTOは内部ID/owner/folder/sourceKeyを投影しない。HTMLのURLは `PREVIEW_ORIGIN/public/{publicId}/html?version=N`。公開画面のURLは `APP_ORIGIN/public/knowledge/{publicId}`。最初のpublicIdを停止後も保持し、再公開に再利用する。

## DBとprivateストレージ

`migrations/000001_knowledge.up.sql` と `down.sql` をgolang-migrateで適用・巻戻す。自動適用や既存DB初期化はAPI起動へ追加していない。owner_id/session_idは#19の安定識別子を受けるtextで、独自のユーザー・認証テーブルを作らない。folderは所有者との複合外部キーで照合する。

[.env.example](.env.example) の `APP_ORIGIN`、`PREVIEW_ORIGIN`、`S3_*` を全て設定し、MinIO/S3にprivate bucketを用意する。未設定時もhealth/readyは従来どおり動く。originはパスなしHTTPS、previewはCookieを隔離できる別hostnameを必須とする。S3へクライアントをredirectせずGo経由で本文を返す。

保存はpending objectのDB記録→行ロック中のPut→ready→知識の行ロック/version照合とactive化・metadata・監査の同一commitで行う。新しい不変keyを使い、旧objectを上書きしない。Put/DB失敗やcommit結果不明で新objectを即削除せず、参照確認付き回収へ残す。draft確定はdraft行ロックと一意制約で同一本文の再送を同ID/200にし、初回201、異なる本文409とする。

回収は同じDB/S3設定で `go run ./cmd/collect-knowledge` を保守実行する。1回最大100件、24時間以上前の未参照objectだけを処理する。draft自体の期限ではない。Put・確定とobject行ロックで排他し、取得後にも新しいREAD COMMITTED照会で参照を再確認する。削除失敗は行を保持し再実行可能にする。CLIは5分の期限付き。APIと異なるbucketを指定しない。

## HTTPと入力境界

管理APIは未認証401、別所有者/不存在404、未知JSONフィールド/ID/version不正400、異Origin403、version競合409、本文10 MiB超過413、本文/front matter違反422、DB/S3障害503を共通error形式で返す。JSON wire本文はエスケープ展開分を含む上限で制限し、decode後のUTF-8本文も10 MiBで検査する。front matterのschema違反はノード位置、YAML構文エラーはパーサが示す行の先頭位置を返し、原文や内部例外をmessageへ出さない。

HTMLは保存時にbluemondayの要素・属性・CSS許可リストで安全化する。CSS長さは非負、整数部4桁/小数部3桁まで、shorthandは最大4値。CSS escape/`@`/`!` を含むstyle属性は解析正規化の前に除去する。style要素/画像/外部通信は許可しない。HTML sourceを上書きせず別objectを配信し、変更があれば `warnings:[{code:"html_sanitized"}]` を返す。安全化の有無は現在の保存版と同じtransactionでDBへ保持し、詳細GET・保存・公開切替の詳細応答で通知する。draft正式保存の応答形状は維持し、遷移後や応答消失回復後の詳細GETから通知できる。安全化されない次版の保存ではfalseへ更新する。

`POST .../preview-ticket` は `{version}` を受けて `{url}` のみを返す。独立expiresAt/60秒猶予はない。URLはログや永続Queryキャッシュへ保存しない。previewホストだけでHTMLを配信し、no-store/no-referrer/nosniffとCSPを付ける。旧version、停止中公開、失効sessionは新規取得を拒否する。プロキシにもquery非記録・キャッシュ無効・previewのGo転送が必要であり、現行のE2E proxyはこの条件でGo APIへ接続済み。

## 検証

`task --dir dev/backend fmt`、`task --dir dev/backend check` を使用する。domain・HTML安全化・HTTP境界・usecaseのテーブル駆動単体テストは通常のGoテストで実行する。

DB結合テストは専用の使い捨てPostgreSQLへ `KNOWLEDGE_TEST_DATABASE_URL` を指定し、`go test ./internal/infrastructure/postgres -run TestKnowledgeIntegration -v` で実行する。テスト専用schemaを作成・削除し、migration往復、並行確定、10年前のdraft保護、公開ID維持、保存競合/失敗、session失効の連動、回収保護を検証する。session/S3は手書きfakeであり、実OAuth/MinIO/3ブラウザE2Eの成功とは区別する。

### 認証を省略する業務E2Eサーバー

業務E2Eは製品の `cmd/api` とは別の `cmd/e2etest` を明示的にビルドして実行する。起動時に `APP_ENV=test` と空でない `E2E_RUN_ID` を要求し、既存migrationの適用とS3 private bucketの存在確認を終えてから `health`/`ready` と知識APIを公開する。Dockerfileは `go build ./cmd/e2etest` をビルド対象にし、migration SQLを `/migrations` へ配置する。通常の製品binaryへテスト認証を混ぜない。

テストserverだけが `__Host-kp_e2e_actor` Cookieの `a` / `b` を、次の固定identityへ変換する。Cookieなし・未知値は未認証として扱う。`GET /api/v1/auth/me` と知識APIの `Authenticate` は同じ対応表を使い、`Sessions.Active` はこの2つの固定sessionを有効とするfakeである。これはOAuth、logout、実session失効の検証ではない。

| Cookie | owner UUID | session UUID | displayName |
| --- | --- | --- | --- |
| `a` | `aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa` | `aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaab` | `E2E Actor A` |
| `b` | `bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb` | `bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbc` | `E2E Actor B` |

fixture作成はテストserver専用の `POST /api/v1/__e2e/fixtures` を使う。`{owner:"a"|"b",format:"markdown"|"html",source:string,ageYears?:number}` を受け、実S3へsourceを保存し、実PostgreSQLの `knowledge_objects` と `knowledge_drafts` を同一transactionで作成して `{draftId:string}` を返す。各呼出しはUUIDと `E2E_RUN_ID` を含むobject keyを生成し、`ageYears:10` は古いdraft保持試験に使える。Markdownの許可されたfront matterは既存 `domain.ApplySource` でmetadataへ反映し、未指定値は最小既定値を使う。endpointはfixture専用環境へ閉じ、upload APIや汎用seed/reset基盤は追加しない。
