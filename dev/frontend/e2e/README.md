# 横断E2E

[横断検証規約](../../../docs/testing.md)に従い、既存SPA・Go API・PostgreSQLをDockerで起動し、実ブラウザでHTTPS疎通を確認する。
現在のテストは環境の疎通を確認するもので、X01〜X07の業務シナリオの完了を意味しない。

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
OAuth代替と公開配信は雛形のため、業務要求へ501を返すことも確認する。ログイン成功や公開閲覧成功として扱わない。

## 未実装部分と実装時の参照先

| 入口                                    | 現在の状態                                                                        |
| --------------------------------------- | --------------------------------------------------------------------------------- |
| `fixtures/knowledge/prepare.ts`         | 入力データを保存してID対応表を返す雛形。未実装エラー                              |
| `fixtures/auth/authenticate.ts`         | 実OAuth callback経由でログインする雛形。未実装エラー                              |
| `environment/init/business-seed.mjs`    | 業務migration・MinIO bucket・fixture投入の雛形。`task e2e:env:seed`は未実装エラー |
| `environment/oauth/server.mjs`          | `/health`のみ応答。認可・code交換・PKCE・ユーザー取得は501                        |
| `environment/proxy/proxy.conf`のpreview | 別ホストとCSPを設定済み。実ファイル配信は501                                      |

配置と増やし方は[ディレクトリ設計](../../../docs/testing.md#e2eのディレクトリ設計)、テストの具体化は[横断シナリオ規約](../../../docs/testing.md#必須の横断シナリオ)を参照する。
作成は[e2e-implement](../../../.agents/skills/e2e-implement/SKILL.md)、レビューは[e2e-review](../../../.agents/skills/e2e-review/SKILL.md)の手順を利用する。

## 補助コマンド

| コマンド             | 使用する場面                                                 |
| -------------------- | ------------------------------------------------------------ |
| `task e2e:install`   | ローカルの一覧確認・静的検査に使う依存を導入                 |
| `task e2e:env:seed`  | 業務データ投入。現時点は未実装エラー                         |
| `task e2e:artifacts` | 残したtestコンテナからレポートを再回収。通常はtestが自動実行 |

## 調査と個別再実行

ローカルの一覧確認は`task e2e:install`の後、`pnpm --dir dev/frontend test:e2e:list`を使う。
ソースはイメージへコピーするため、変更後は`task e2e:env:up`で再ビルドする。
起動・初期化済み環境で個別実行する場合は次を使う。

```bash
COMPOSE_PROJECT_NAME="knowledge-e2e-$E2E_RUN_ID" docker compose -f dev/compose.test.yaml run --rm --no-deps test pnpm run test:e2e e2e/tests/environment.spec.ts
```

通常はコンテナ内DNSを使い、ホストへポートを公開しない。ホストのブラウザで調査する必要が生じた場合は、proxyのlocalhost限定ポート公開・hosts・CA導入手順を別途追加する。個人端末の信頼ストアを自動変更しない。

## 動画の確認

ブラウザ操作のあるテストは成功・失敗を問わず動画を保存する。`artifacts/report/index.html`でテストを開き、動画の添付から再生する。動画ファイルは`artifacts/results/`にも保存される。APIリクエストだけのテストにはブラウザ動画は作られない。

動画も固定のartifacts配下へ保存し、次回回収時に前回分と置き換える。Firefoxは共通fixtureが録画とレポートへの添付を行う。現在のFirefox fixtureの録画モードはon/offに対応する。秘密値を表示するテストでは、規約に従って`test.use({ video: "off" })`を指定する。
