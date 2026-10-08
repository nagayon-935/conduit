# Conduit — Web SSH Terminal

[![CI](https://github.com/nagayon-935/Conduit/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/nagayon-935/Conduit/actions/workflows/ci.yml)
[![Coverage Status](https://coveralls.io/repos/github/nagayon-935/conduit/badge.svg?branch=main)](https://coveralls.io/github/nagayon-935/conduit?branch=main)
[![Go Version](https://img.shields.io/badge/Go-1.25-00ADD8?logo=go&logoColor=white)](https://go.dev/doc/go1.25)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

> ⚠️ **開発中 (Work in Progress)**
> このプロジェクトは現在開発中であり、実環境での動作確認は行っていません。
> 本番環境での使用は推奨しません。

ブラウザから SSH に接続できる Web ターミナルアプリケーションです。
HashiCorp Vault が発行する短命 SSH 証明書（TTL=5分）で認証し、WebSocket 経由でリアルタイムにターミナルを操作できます。

**`conduit-cli`** により、ローカルのターミナルからも同じ Vault 証明書認証で SSH 接続できます。

---

## アーキテクチャ

```text
Browser (/login, /app, /admin)       Local terminal (conduit-cli)
    │ Cookie + CSRF                         │ Vault + native ssh
    ▼                                      ▼
Go server                         HashiCorp Vault
    ├─ /api/auth/*                    SSH Secrets Engine
    ├─ /api/app/*    (所有者・対象の権限)
    ├─ /api/admin/*  (管理者の権限)
    └─ /ws?ticket=… (Cookie に紐づく一回限りチケット)
         │
         ├─ SQLite (利用者・権限・履歴・監査)
         ├─ asciinema .cast (所有者付き録画)
         └─ 登録済み SSH 接続先 (任意で 1 段の踏み台経由)
```

Conduit のログインと SSH 認証を分離しています。同じ SSH 名を使う利用者同士でも、端末・履歴・録画は別の所有者として扱います。管理者画面では接続先と SSH アカウントを登録し、利用者に接続・共有閲覧の権限を付与します。管理者も自身への接続許可が必要です。

初回管理者作成、既存データ移行、復旧、権限・期限・共有の詳細は [導入手順](docs/admin-user-implementation.md)、設計の根拠は [分離設計](docs/admin-user-design.md) を参照してください。

## 技術スタック

### バックエンド

- **Go 1.25**
- `golang.org/x/crypto/ssh` — SSH クライアント・証明書認証
- `github.com/gorilla/websocket` — WebSocket サーバー
- `modernc.org/sqlite` — 利用者・権限・履歴・監査の永続化（cgo 不要の Pure Go SQLite）
- HashiCorp Vault HTTP API — SSH 証明書署名
- asciinema v2 (`.cast`) — セッション録画フォーマット

### フロントエンド

- **React 18 + TypeScript**
- `@xterm/xterm` — ターミナルエミュレータ（WebGL レンダラー）
- `@xterm/addon-fit` — ウィンドウサイズ自動追従
- `@xterm/addon-webgl` — GPU アクセラレーション描画
- `@xterm/addon-search` — ターミナル内検索
- `asciinema-player` — 録画再生プレイヤー
- Vite 5 — ビルドツール・開発サーバー

### CLI

- **Cobra** — コマンドラインインターフェース
- **Viper** — 設定ファイル・環境変数読み込み

---

## ディレクトリ構成

```text
.
├── cmd/
│   ├── server/          # Web サーバーのエントリポイント
│   └── cli/             # conduit-cli のエントリポイント
├── internal/
│   ├── api/             # HTTP ハンドラー (connect, terminal, sessions, share, logs, recordings)
│   ├── cli/             # conduit-cli の設定・ssh 実行ロジック
│   ├── config/          # 環境変数設定・シークレット型
│   ├── control/         # 永続 DB・本人確認・利用者・対象・権限・監査
│   ├── connlog/         # 旧ログ形式（移行用）
│   ├── recording/       # asciinema v2 録画レコーダー
│   ├── session/         # 所有者付き SSH 状態・GC
│   ├── sshconn/         # 鍵生成・SSH ダイアル・ProxyJump・証明書サイナー
│   ├── tunnel/          # WebSocket↔SSH ポンプ・PTY リサイズ・バックプレッシャー
│   └── vault/           # Vault クライアント
├── pkg/token/           # セッショントークン生成
├── tests/               # E2E 統合テスト
└── frontend/            # React フロントエンド
    └── src/
        ├── portal/      # ログイン・ユーザー作業場・管理者画面
        ├── api/         # Cookie・CSRF 対応 REST クライアント
        ├── components/  # ConnectForm, Terminal, TabBar, SessionList, LogPage, NewConnectionOverlay
        ├── hooks/       # useTerminal, useWebSocket, useProfiles, useConnectionHistory
        ├── themes/      # ターミナルカラーテーマ
        ├── utils/       # crypto, storage, parseSshConfig など
        └── types/       # 型定義
```

---

## 本番デプロイ

詳細は [DEPLOY.md](DEPLOY.md) を参照してください。

### 接続先 SSH サーバーのセットアップ

Conduit から接続したい SSH サーバーで以下のスクリプトを実行します：

```bash
curl -fsSL https://raw.githubusercontent.com/nagayon-935/Conduit/main/scripts/setup-ssh-server.sh \
  | bash -s http://<VaultのIP>:8200
```

またはリポジトリをクローンしている場合：

```bash
bash scripts/setup-ssh-server.sh http://<VaultのIP>:8200
```

スクリプトが行うこと：

1. Vault から CA 公開鍵を取得し `/etc/ssh/trusted-ca.pub` に保存
2. `/etc/ssh/sshd_config` に `TrustedUserCAKeys` を追記
3. `sshd` を再読み込み

---

## ユーザーガイド

### ログインと接続

管理者から受け取った Conduit アカウントでログインし、初回はパスワードを変更します。「接続先」で許可済みの接続先を検索・選択します。必要な SSH パスワード・秘密鍵・パスフレーズだけを入力します。Vault 証明書方式では追加の秘密情報入力は不要です。SSH 資格情報をブラウザや DB に保存しません。

### 作業場・セッション・共有

複数の端末をタブや分割配置で利用できます。履歴や管理画面へ移動しても端末は維持します。公開 ID と配置をユーザーごとに保存し、再読み込み時は所有権と生存状態を確認して再接続します。

タブを閉じても SSH は既定で 15 分間保持されます。「自分のセッション」の「終了」は SSH プロセスを終了します。ログアウト時にもすべての自分の SSH を終了するか選択できます。

共有は指定ユーザーへの読み取り専用です。閲覧者もログインが必要で、リンクだけでは閲覧できません。共有の停止・期限・権限取り消しは既存の閲覧接続にも反映します。

「接続履歴」は自分の履歴と録画、「表示設定」は個人のテーマ・文字サイズです。管理者は管理画面で全体の履歴・録画・監査を確認し、理由を記録してセッションや共有を停止できます。

---

## ターミナル操作

### キーボードショートカット

| ショートカット | 機能 |
|---------------|------|
| `Ctrl` + `=` | フォントサイズを拡大 |
| `Ctrl` + `-` | フォントサイズを縮小 |
| `Ctrl` + `F` | ターミナル内検索を開く / 閉じる |
| `Enter` | 次の検索結果へ |
| `Shift` + `Enter` | 前の検索結果へ |
| `Escape` | 検索を閉じる |

テーマと文字サイズは Conduit ユーザーの設定として DB に保存されます。ショートカットは選択中の端末に適用します。

---

## conduit-cli

`conduit-cli` はローカルターミナルから Vault 証明書認証で SSH 接続できるコマンドラインツールです。
Web UI と同じ Vault 設定・ロールを使用し、一時的な SSH 証明書を取得してネイティブの `ssh` コマンドを実行します。

### ビルド

```bash
make build-cli
# → bin/conduit-cli
```

### 必要な環境変数

| 変数名 | 必須 | 説明 |
|--------|------|------|
| `VAULT_ADDR` | ✅ | Vault サーバーのアドレス |
| `VAULT_TOKEN` | ✅ | 発行者（ユーザーまたはサービス）ごとの Vault トークン |
| `VAULT_SSH_ROLE` | ✅ | SSH 証明書署名に使用するロール名 |
| `VAULT_SSH_MOUNT` | | Vault SSH Secrets Engine のマウントパス（デフォルト: `ssh`） |

### 設定ファイル

`$XDG_CONFIG_HOME/conduit/config.yaml` または `~/.config/conduit/config.yaml` に YAML 形式で設定できます。
`VAULT_TOKEN` は環境変数経由でのみ渡すことを推奨します。

```yaml
vault:
  addr: "https://vault.example.com:8200"
  ssh_mount: "ssh"

ssh:
  default_auth: "vault"        # vault | password | pubkey
  vault_role: "conduit-cli-role"
  known_hosts: "$HOME/.ssh/known_hosts"
  port: 22

log:
  level: "silent"              # silent | info | debug
```

設定値の優先順位:

1. コマンドラインフラグ
2. 環境変数
3. 設定ファイル
4. デフォルト値

### 使用例

```bash
# Vault 証明書認証（デフォルト）
VAULT_ADDR=https://vault.example.com:8200 \
VAULT_TOKEN=s.xxx \
VAULT_SSH_ROLE=conduit-cli-role \
  ./bin/conduit-cli ssh user@host

# ポート指定
./bin/conduit-cli ssh -p 2222 user@host

# 踏み台（ProxyJump）経由
./bin/conduit-cli ssh -J admin@bastion:2222 user@target.internal

# パスワード認証
./bin/conduit-cli ssh -A password user@host

# 公開鍵認証
./bin/conduit-cli ssh -A pubkey -i ~/.ssh/id_ed25519 user@host

# リモートコマンド実行
./bin/conduit-cli ssh user@host -- ls -la

# 詳細ログ表示
./bin/conduit-cli ssh -vv user@host
```

### 終了コード

| コード | 意味 |
|--------|------|
| `0` | 正常終了 |
| `ssh` の終了コード | `ssh` コマンド自体が返したコード |
| `1` | 引数・設定ファイル・環境変数のエラー |
| `2` | Vault 署名エラー |
| `3` | 鍵生成エラー |
| `4` | `ssh` コマンドが見つからない |
| `130` | ユーザーによる中断（Ctrl+C） |

---

## ローカル開発セットアップ

### 前提条件

- Go 1.25+
- Node.js 18+
- HashiCorp Vault（SSH Secrets Engine 有効化済み）

### 環境変数

| 変数 | 既定値 | 内容 |
| --- | --- | --- |
| `VAULT_ADDR` / `VAULT_TOKEN` / `VAULT_SSH_ROLE` | 必須 | サーバー側の Vault 連携 |
| `VAULT_SSH_MOUNT` | `ssh` | SSH Secrets Engine |
| `SERVER_PORT` | `8080` | API リッスンポート。Vite の既定プロキシ先は `8888` |
| `PUBLIC_URL` | リクエストの同一ホスト | ブラウザで使うオリジン。本番は HTTPS、Vite は `http://localhost:5173` を明示 |
| `CONDUIT_DEV_HTTP` | 無効 | `true` で明示的な HTTP 開発用 Cookie・ホスト鍵未設定の例外 |
| `KNOWN_HOSTS_PATH` | 本番は必須 | 接続先と踏み台の known_hosts |
| `SSH_ALLOWED_CIDRS` | 空（全拒否） | SSH 接続先・踏み台の許可ネットワーク（カンマ区切り） |
| `TRUSTED_PROXY_CIDRS` | 空 | `X-Real-IP` を信用するプロキシの送信元だけを指定 |
| `DB_PATH` | `./data/conduit.db` | 永続 SQLite。Web のメモリストアは廃止 |
| `GRACE_PERIOD` | `15m` | 初期の SSH 再接続猶予 |
| `SESSION_GC_INTERVAL` | `1m` | SSH の GC 間隔 |
| `SESSION_IDLE_TIMEOUT` | `30m` | 初期の SSH 無入力期限。`0` で無効 |
| `RECORDING_ENABLED` | 無効 | 初期の録画設定 |
| `RECORDING_DIR` | `./recordings` | 録画ディレクトリ（DB と合わせて永続化） |

期限・録画・保持期間は管理画面の運用設定で変更できます。DB に保存済みの運用設定が初期環境変数より優先します。

初期管理者作成と具体的な起動例は [導入手順](docs/admin-user-implementation.md#初回セットアップ) を参照してください。

### バックエンド起動

```bash
# 依存パッケージ取得
go mod download

# ビルド & 起動
make build
make run

# または開発モード（go run）
make dev
```

### フロントエンド起動

```bash
cd frontend
npm install
npm run dev   # http://localhost:5173 で起動
```

> Vite の既定プロキシ先は `localhost:8888` です。`SERVER_PORT=8888` と `PUBLIC_URL=http://localhost:5173` を設定してください。
> Vite の開発サーバーが `/api` と `/ws` を自動プロキシします。

### CLI ビルド

```bash
make build-cli
# または両方まとめて
make build-all
```

---

## テスト

```bash
# 全テスト（レースディテクター付き）
make test

# カバレッジレポート
go test -covermode=atomic -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

フロントエンドは `cd frontend && npm test && npm run build` で検証します。Go の統合テストでは Cookie ログイン、対象登録、grant、Vault 署名、実際の SSH 端末入出力・再接続を通して確認します。

---

## API

`/api/auth/*` はログイン・パスワード・ログアウト、`/api/app/*` は所有する接続・履歴・共有、`/api/admin/*` は管理者によるユーザー・対象・grant・運用管理です。変更 API は Cookie、`X-CSRF-Token`、同一オリジンの Origin が必要です。

接続作成は `POST /api/app/sessions` に `target_account_id` と今回の `credentials` / `jump_credentials` だけを送ります。ホスト・SSH 名・認証方式・踏み台の上書きは拒否します。レスポンスの `id` は公開 ID であり、接続能力を持つトークンではありません。

端末は所有者・共有宛先の権限を検証して `/ws-ticket` から 30 秒のチケットを発行し、Cookie とともに `/ws?ticket=...` で一回限り使用します。Binary frame は端末入出力、Text frame は ping/resize の JSON 制御です。

`GET /healthz` は公開の最小 liveness です。旧匿名 API・旧 WS token/share 接続は廃止しました。詳細な API と権限モデルは [設計書](docs/admin-user-design.md#9-api-設計) を参照してください。

---

## ライセンス

MIT
