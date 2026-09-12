---
name: e2e-implement
description: knowledge-previewのdev/frontend/e2eでPlaywrightのブラウザE2E・実API契約テスト・fixtureを作成、修正する。E2Eに必要なテスト環境の変更も扱う。レビューのみ、Vitestの単体テストのみ、製品機能だけの実装には使用しない。
---

# E2Eの作成・修正

対象Issueで決めたシナリオを、実SPA・Go API・DBに接続して検証できるテストへ実装する。日本語で意図を説明する。

## Ponytailとの併用

実装時は [ponytail](../ponytail/SKILL.md) を併用し、既存実装・標準機能を優先する。明示されたモードを尊重する。ユーザーがPonytailを停止している場合は適用しない。簡略化のために、対象Issueの要件、本スキルの責務境界、認可・安全性、必須検証を省略しない。

## 最初に読むもの

- 対象Issueの受け入れ基準と具体的なシナリオ、対象差分。`git status --short`で既存の変更・未追跡ファイルを確認する。
- [横断検証規約](../../../docs/testing.md)：必須シナリオ、責務、ディレクトリ設計、回帰・CI完了条件。
- [E2E README](../../../dev/frontend/e2e/README.md)、[ルートTaskfile](../../../Taskfile.yml)：現在動く範囲と実行方法。
- [Playwright設定](../../../dev/frontend/playwright.config.ts)、[E2E型設定](../../../dev/frontend/tsconfig.e2e.json)、[共通test fixture](../../../dev/frontend/e2e/fixtures/test.ts)、[依存](../../../dev/frontend/package.json)、近い既存spec。

環境を変更するときは[Compose](../../../dev/compose.test.yaml)と該当Dockerfile・起動スクリプトも読む。製品の振る舞いを確認するときは対応する両層の実装・API契約まで追う。

## シナリオからテストへ

1. 共通のXシナリオだけで実装を始めず、ユーザー権限・初期状態・入力・操作・実APIの結果・再取得後の表示・該当する失敗条件を対象Issueの仕様と対応付ける。未確定のAPIや成功条件を推測しない。実装を左右する不明点は具体的にすり合わせ、回答に依存しない準備は進める。GitHubへの記載・コメントは依頼された場合に行う。
2. テスト名は条件と期待結果を日本語の一文にし、管理IDを接頭辞にしない。Issue・規約との対応はコメントへ記載する。ブラウザ操作は`tests/`、HTTP契約は`contracts/`へ配置する。分割と共通化は規約の基準を使い、将来用の空ディレクトリを作らない。ブラウザspecは`fixtures/test.ts`からtest・expectをimportし、FirefoxのCAとprofileの準備・解放を維持する。
3. fixtureの入力と保存済みIDを区別し、実行・テストごとにデータを分離する。既存のprepare・authenticateが未実装なら、そのエラーを握りつぶして進めない。必要な実装が対象範囲に入らない場合は、何が不足しどの検証ができないかを報告する。
4. UI操作はrole・ラベルなど利用者が識別できるlocatorを優先する。Playwrightの待機付きassertionや必要な応答を待ち、固定sleep・force操作・広すぎるlocatorで不安定さを隠さない。応答を待つ場合はトリガー操作より前に待機を登録する。
5. 成功トーストだけで保存を判定せず、再読込・実APIの再取得など、対象の層を接続した結果を確認する。製品APIの正常応答をモックして横断テスト成功としない。失敗注入は観測対象と差替え範囲を明示し、実接続の検証と区別する。
6. 認証・公開・トークンを扱う変更では規約の該当条件を確認する。Cookie注入で実OAuthを迂回したり、TLS検証を無効化したりしない。秘密値をassertionの差分・ログ・添付へ露出させない。

一つの問題を直すために全シナリオや環境を作り直さない。製品コードの変更が必要なら、依頼範囲を確認し、対象層の実装規約に従う。

## 検証

変更対象の整形・Lint修正後、静的検査と型検査を行う。コマンドは現在のpackage.json・Taskfileで確認する。

```bash
task e2e:install
pnpm --dir dev/frontend lint
pnpm --dir dev/frontend fmt:check
pnpm --dir dev/frontend typecheck:e2e
pnpm --dir dev/frontend test:e2e:list
```

テスト一覧に新規specが含まれていることを確認する。一覧表示は実行成功ではない。ソースはDockerイメージにCOPYするため、変更後に再ビルドし、[実行手順](../../../dev/frontend/e2e/README.md)から該当シナリオを実行する。環境・共有fixture・配信設定の変更は全対象ブラウザと関連回帰を検証する。

一括実行は実行固有のE2E_RUN_IDで`task e2e:check`を使う。個別実行は調査が終わったら同じIDでdownを行う。他の実行の環境を削除しない。起動・初期化・テストの失敗をcleanup成功で上書きせず、環境変更ではプロファイル対象を含めコンテナ・volumeが残らないことも確認する。

0件・skip・未実装エラーを成功にしない。失敗時は原因を調べ、retryやtimeoutを増やすだけで通さない。ツール不足やDocker停止はコード不具合と区別し、実行できていない項目を報告する。

## 完了時

作成したシナリオと観測結果、検証したブラウザ、実行結果、残る未実装・未検証を簡潔に伝える。環境疎通をX01〜X07の業務検証成功として扱わない。根拠には実際の記述・結果とファイルの絶対パス・行番号を添える。コミット・PR・外部投稿はその依頼がある場合に行う。
