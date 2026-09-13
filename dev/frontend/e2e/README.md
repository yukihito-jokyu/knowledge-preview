# 横断E2E

[横断検証規約](../../../docs/testing.md)に従い、既存SPA・Go API・PostgreSQLをDockerで起動し、実ブラウザでHTTPS疎通を確認する。
環境疎通に加え、認証をテスト用fixtureで代替した知識編集・draft正式保存・競合・HTML隔離を検証する。実OAuthや他Issueの未実装画面の完成を意味しない。

## 手動で画面を操作する

リポジトリルートで次を実行する。Task v3、Docker Compose v2、Node.js 24.9以降、pnpmが必要。`manual:open`が固定依存とPlaywright固定版Chromiumを初回にダウンロードする。

```bash
task manual:up     # 構築・起動・疎通確認。E2E_RUN_IDの指定は不要
task manual:open   # 専用ChromiumでMarkdownとHTMLのdraft編集画面を開く
# 操作が終わったらブラウザを閉じる（またはCtrl+C）
task manual:down   # サービスを停止。保存データは残る
```

専用Chromiumはテスト所有者Aで認証済みになる。各画面の「正式保存」から保存・編集・公開操作を試せる。HTMLは正式保存後にプレビューを表示する。保存した知識はサイドバーの「最近のファイル」から開ける。`manual:open`を実行するたびにMarkdownとHTMLのサンプルdraftを各1件追加する。

HTTPSの公開先は`127.0.0.1:443`だけで、DBやMinIOはホストへ公開しない。443番を使用中のサービスがある場合は、そのサービスを停止してから起動する。`https://app.knowledge.test`と`https://preview.knowledge.test`の名前解決・CA信頼は専用Chromiumの一時profileだけに設定するため、通常のブラウザから直接URLを開く用途ではない。OSのhosts・キーチェーンや普段のChrome設定は変更しない。TLS検証は有効で、認証はE2Eと同じテスト用Cookieを利用する。

手動環境はCompose project `knowledge-manual`を使い、自動E2Eの実行IDや動画レポートとは分離する。`manual:down`やブラウザ終了でDB・MinIOのデータは削除しない。再開・コード変更後は`manual:up`→`manual:open`を実行する。`manual:up`は証明書を更新するため、実行前に専用ブラウザを閉じる。テストの`e2e:check`や`e2e:env:down`を手動環境の停止には使わない。

コマンド自体の動作確認には、起動後に`task manual:check`を実行する。専用Chromiumをヘッドレスで起動し、2形式のdraft作成・正式保存・再読込・HTML隔離表示を実API/DB/MinIOで確認して終了する。この確認もサンプルを各1件保存する。実OAuth・ログアウト・アップロード入口・検索一覧・公開専用画面は対象外。

## 準備と実行

Task v3、稼働中のDocker、Docker Compose v2が必要。イメージは`environment/**/Dockerfile*`と[Compose](../../compose.test.yaml)でdigestまで固定しており、`.env`のコピーやイメージの手動指定は不要。
依存とnpmスクリプトは`dev/frontend/package.json`へ統合している。コマンドはリポジトリルートで実行する。

```bash
export E2E_RUN_ID="local-$(date +%s)-$$"
task e2e:config     # 実行IDとCompose定義を検証
task e2e:env:up     # ビルド、実行専用CAの生成、サービス起動
task e2e:env:init   # SPA・API・DBと雛形サービスの疎通確認
task e2e:test       # Playwright実行、成否にかかわらず成果物を回収
task e2e:env:down   # 当該実行のコンテナ・DB・証明書・成果物volumeを削除
```

一括実行は、同じE2E_RUN_IDを指定して`task e2e:check`を使う。失敗時も後片付けを試み、最初の失敗を保持する。個別コマンドは環境を残すので、失敗時もdownを実行する。再初期化する場合もdownからやり直す。

CIは実行ごとに一意なE2E_RUN_IDを設定し、一括コマンドを使う。強制停止に備え、CI側にも常時実行のdownを置く。横断CIのworkflow自体は未実装。

成果物は固定の`dev/frontend/e2e/artifacts/`へDockerのコピー機能で回収する。回収前に前回分を削除し、最新の`report/`・`results/`だけを残す。実行ID別のフォルダは作らない。同じチェックアウトで複数実行した場合は最後に回収した結果で置き換わるため、成果物回収を同時に実行しない。ホストのディレクトリをコンテナにmountしない。回収済みファイルはdownで削除しない。CIの保持期間は7日。認証Cookie・code・トークンなどの秘密値をログや成果物へ出力しない。

## 現在の検証範囲

環境疎通テストはChromium・Firefox・WebKitでログイン画面の表示と、実DBへPingする`/api/ready`の204を確認する。実行専用CAを信頼させ、TLS検証は有効のままにする。
OAuth代替は雛形のため、業務要求へ501を返すことも確認する。知識のHTML配信は実APIへ接続する。テスト用認証を実OAuthログイン成功として扱わない。

## 未実装部分と実装時の参照先

| 入口                                    | 現在の状態                                                                    |
| --------------------------------------- | ----------------------------------------------------------------------------- |
| `fixtures/knowledge/prepare.ts`         | テスト専用fixture APIで実DB/MinIOにdraftを作成する                            |
| `fixtures/auth/authenticate.ts`         | 実OAuth callback経由でログインする雛形。未実装エラー                          |
| `environment/init/business-seed.mjs`    | 起動時にmigration/bucketを準備し、各テストがfixture APIで投入する旨を案内する |
| `environment/oauth/server.mjs`          | `/health`のみ応答。認可・code交換・PKCE・ユーザー取得は501                    |
| `environment/proxy/proxy.conf`のpreview | 別ホストから実Go APIのHTML配信へ接続する                                      |

配置と増やし方は[ディレクトリ設計](../../../docs/testing.md#e2eのディレクトリ設計)、テストの具体化は[横断シナリオ規約](../../../docs/testing.md#必須の横断シナリオ)を参照する。
作成は[e2e-implement](../../../.agents/skills/e2e-implement/SKILL.md)、レビューは[e2e-review](../../../.agents/skills/e2e-review/SKILL.md)の手順を利用する。

## 補助コマンド

| コマンド             | 使用する場面                                                 |
| -------------------- | ------------------------------------------------------------ |
| `task e2e:install`   | ローカルの一覧確認・静的検査に使う依存を導入                 |
| `task e2e:env:seed`  | 各テストがfixture APIでデータを作ることを案内する            |
| `task e2e:artifacts` | 残したtestコンテナからレポートを再回収。通常はtestが自動実行 |

## 調査と個別再実行

ローカルの一覧確認は`task e2e:install`の後、`pnpm --dir dev/frontend test:e2e:list`を使う。このコマンドはテストを実行せず、HTMLレポートと`results/`の実行成果物も更新しない。
ソースはイメージへコピーするため、変更後は`task e2e:env:up`で再ビルドする。
起動・初期化済み環境で個別実行する場合は次を使う。

```bash
COMPOSE_PROJECT_NAME="knowledge-e2e-$E2E_RUN_ID" docker compose -f dev/compose.test.yaml run --rm --no-deps test pnpm run test:e2e e2e/tests/environment.spec.ts
```

自動E2Eはコンテナ内DNSを使い、ホストへポートを公開しない。ホストで操作するときは上記の`manual:*`を使う。個人端末の信頼ストアは自動変更しない。

## 動画の確認

ブラウザ操作のあるテストは成功・失敗を問わず動画を保存する。`artifacts/report/index.html`でテストを開き、動画の添付から再生する。動画ファイルは`artifacts/results/`にも保存される。APIリクエストだけのテストにはブラウザ動画は作られない。

動画も固定のartifacts配下へ保存し、次回回収時に前回分と置き換える。Firefoxは共通fixtureが録画とレポートへの添付を行う。現在のFirefox fixtureの録画モードはon/offに対応する。秘密値を表示するテストでは、規約に従って`test.use({ video: "off" })`を指定する。
