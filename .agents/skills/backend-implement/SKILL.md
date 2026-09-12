---
name: backend-implement
description: dev/backendのGoバックエンドを実装・修正・リファクタリングする。API、Use case、レスポンス、PostgreSQL接続などを既存構成に沿って変更し、Taskfileで検証する場合に使用する。レビューのみの依頼やフロントエンドだけの変更には使用しない。
---

# バックエンド実装

## 最初に確認すること

パスはリポジトリルートを基準に扱う。以下のリンクはこのスキルからの相対パスである。

- [ARCHITECTURE.md](../../../dev/backend/ARCHITECTURE.md) で責務・依存方向・配置を確認する。
- [Taskfile.yml](../../../dev/backend/Taskfile.yml)、[.golangci.yml](../../../dev/backend/.golangci.yml)、[go.mod](../../../dev/backend/go.mod) と [.golangci-lint-version](../../../dev/backend/.golangci-lint-version) で実行方法とツール要件を確認する。バージョンを記憶から決めない。
- `git status --short` と差分を確認し、既存の未コミット変更を把握する。未追跡ファイルも対象に含め、依頼に必要な箇所だけ変更する。
- 対象コード、呼び出し元、関連テストを読む。仕様と実装の差を切り分け、現在のユーザー指示を優先する。

## 配置と実装の判断

現在の基本配置は次のとおり。構成が更新されている場合はARCHITECTURE.mdと実コードに合わせる。

| 対象 | 配置 |
| --- | --- |
| 起動・依存の組立て | `cmd/api/main.go` |
| 環境設定 | `internal/config` |
| 業務ルール・エラー | `internal/domain` |
| 処理の流れ | `internal/usecase/<機能名>.go` |
| HTTPの受付 | `internal/interface/http/<機能名>.go` |
| レスポンスの型・出力 | `internal/interface/response` |
| PostgreSQL固有の実装 | `internal/infrastructure/postgres` |

- 機能を追加するだけでサブディレクトリを作らない。`persistence` のような分類だけの中間階層や、HTTPハンドラーの `_handler` 接尾辞を増やさない。
- Use caseへGinやpgxの具体型を持ち込まず、必要な操作のinterfaceと `context.Context` を使う。既存の型がinterfaceを満たすなら、目的のないラッパーを追加しない。
- HTTPハンドラーから共通エラーを返す場合は `response.WriteError` を利用する。HTTPステータス、JSON形式、内部エラーの扱いを変更する場合はAPI契約への影響も確認する。
- DB接続や外部呼出しを変更する場合は、失敗時の終了・解放、期限、キャンセルの伝播を確認する。SQLやtransactionを追加する場合にだけ、その変更に必要な整合性を検討する。
- リネーム・移動ではパッケージ名、import、呼出し元、テスト、文書のリンクまで更新する。責務や配置を変えた場合はARCHITECTURE.mdにも反映する。

## 検証と完了

振る舞いを変更した場合は、変更した契約の成功・失敗・境界条件を検証する。HTTPは `httptest`、外部依存は既存interfaceを満たすfakeをまず検討する。単純な移動・改名のためだけにテストを増やさない。

Goコードの変更後はリポジトリルートで順に実行する。

```sh
task --dir dev/backend fmt
task --dir dev/backend check
```

`fmt` はファイルを書き換えるため、実行後に依頼外の変更が混ざっていないか差分を確認する。`check` はLint・整形検査と全Goテストを実行する。
文書だけの変更では、リンクと記述を確認し、Goの検査は必要性に応じて選ぶ。

検査を通すためにLint・除外・テストを緩和しない。必要な抑制にはルール名と具体的な理由を記載する。
ツール不足やキャッシュの権限エラーはコード不具合と区別し、実行環境を確認する。検証用キャッシュを一時ディレクトリへ移す場合はコマンドの環境変数で指定し、個人環境のパスをリポジトリに保存しない。

完了時は変更後の動作、主要なファイル、検証結果、未検証項目を簡潔に報告する。根拠を示す場合は実際のコードの短い引用と絶対パス・行番号のリンクを添える。コミット・PR作成はこのスキルの実行だけでは行わない。
