# Conduit — Web SSH Terminal

[![CI](https://github.com/nagayon-935/Conduit/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/nagayon-935/Conduit/actions/workflows/ci.yml)
[![Coverage Status](https://coveralls.io/repos/github/nagayon-935/conduit/badge.svg?branch=main)](https://coveralls.io/github/nagayon-935/conduit?branch=main)
[![Go Report Card](https://goreportcard.com/badge/github.com/nagayon-935/conduit)](https://goreportcard.com/report/github.com/nagayon-935/conduit)
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
Browser (xterm.js)           Local Terminal (conduit-cli)
    │                              │
    │  WebSocket (binary frames)   │  Vault signing + native ssh
    ▼                              ▼
Go HTTP Server              HashiCorp Vault
    ├─ POST /api/connect              SSH Secrets Engine
    ├─ GET  /ws?token=…
    ├─ GET  /ws?share=…
    ├─ GET  /api/sessions
    ├─ POST /api/sessions/{t}/share
    ├─ GET  /api/logs
    └─ GET  /api/recordings/{id}
         │                                   │
         ▼                                   ▼
    Target SSH Server  (証明書認証)      接続ログ / 録画ストア
   （任意で ProxyJump 踏み台経由）       （SQLite / メモリ・.cast ファイル）
```

### 主要な設計ポイント

| 機能 | 詳細 |
|------|------|
| **短命 SSH 証明書** | Vault SSH Secrets Engine で TTL=5分の証明書を発行。秘密鍵はメモリ上のみに保持しディスクに書かない |
| **グレース期間再接続** | WebSocket 切断後 15 分間は SSH セッションを保持。同じトークンで再接続すると続きから操作できる |
| **バックプレッシャー** | SSH → クライアント方向のチャンネルが詰まった場合、50ms 待って送れなければドロップ。ゴルーチンのフリーズを防ぐ |
| **ProxyJump（踏み台）** | 接続先の手前に踏み台ホストを指定して多段 SSH 接続を確立。踏み台側も Vault / Password / Public Key 認証に対応 |
| **セッション録画** | `RECORDING_ENABLED` 有効時、ターミナル出力を asciinema v2 (`.cast`) 形式で記録。接続ログ画面から再生できる |
| **接続ログの永続化** | 接続履歴を SQLite に保存（`DB_PATH` 未設定時はメモリ）。失敗した接続もエラー付きで記録 |
| **共有セッション（閲覧専用）** | 共有トークンを発行すると、第三者が読み取り専用ビューアとして同じセッションをリアルタイム閲覧できる |
| **conduit-cli** | ローカルターミナルから Vault 証明書認証で SSH 接続できる CLI クライアント |

---

## 技術スタック

### バックエンド

- **Go 1.25**
- `golang.org/x/crypto/ssh` — SSH クライアント・証明書認証
- `github.com/gorilla/websocket` — WebSocket サーバー
- `modernc.org/sqlite` — 接続ログ永続化（cgo 不要の Pure Go SQLite）
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
│   ├── connlog/         # 接続ログストア (SQLite / メモリ)
│   ├── recording/       # asciinema v2 録画レコーダー
│   ├── session/         # セッション状態管理・GC・共有トークン
│   ├── sshconn/         # 鍵生成・SSH ダイアル・ProxyJump・証明書サイナー
│   ├── tunnel/          # WebSocket↔SSH ポンプ・PTY リサイズ・バックプレッシャー
│   └── vault/           # Vault クライアント
├── pkg/token/           # セッショントークン生成
├── tests/               # E2E 統合テスト
└── frontend/            # React フロントエンド
    └── src/
        ├── api/         # REST クライアント (connect, sessions, fetch)
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

### SSH 接続

ブラウザで Conduit を開くと接続フォームが表示されます。
Host / Port / User を入力し、認証方式を選択して **Connect** を押します。

#### 認証方式

| 方式 | 対象 | 必要な入力 |
|------|------|-----------|
| **Vault**（デフォルト） | Vault CA を信頼するよう設定済みのサーバー | なし（証明書は自動発行） |
| **Password** | パスワード認証を許可する任意の SSH サーバー・NW機器 | パスワード |
| **Public Key** | 公開鍵認証を許可する任意の SSH サーバー | 秘密鍵（PEM 貼り付けまたはファイル選択） |

> パスワードはブラウザに保存されません。秘密鍵は、その場限りの接続では保存されませんが、プロファイルとして保存した場合のみ暗号化して localStorage に保持されます。

#### 複数ホストへの同時接続

**+ Add host** ボタンで接続先を追加すると、**Connect All** で全ホストへ並列接続してスプリット表示できます。

#### 踏み台（ProxyJump）経由の接続

接続フォームの **Jump Host** に踏み台ホストを入力すると、その踏み台を経由して目的のサーバーへ多段 SSH 接続します。
踏み台側の認証方式（Vault / Password / Public Key）も個別に指定できます。Jump Host を空にすると踏み台は使用されません。

---

### プロファイル

よく使う接続先をプロファイルとして保存できます。

- **保存**: フォーム入力後、**+ Save as Profile** からプロファイル名を入力して保存
- **読み込み**: Profiles リストのプロファイルをクリックすると Host・Port・User・認証方式・踏み台設定が自動入力
- **Import**: **Import ~/.ssh/config** ボタンで `~/.ssh/config` ファイルを選択すると一括インポート
- **記憶**: 一度接続した認証方式はプロファイル・履歴に記録され、次回選択時に自動で切り替わる

---

### タブ・レイアウト

接続中は画面上部のタブバーで複数セッションを管理できます。

| 操作 | 方法 |
|------|------|
| 新規接続 | **+** ボタン |
| タブ切り替え | タブをクリック |
| タブ並び替え | タブをドラッグ＆ドロップ |
| タブを閉じる | タブ内の **✕** ボタン |
| 左右分割 | レイアウトボタン（⊞）から選択 |
| 上下分割 | レイアウトボタン（⊞）から選択 |
| 2×2 グリッド | レイアウトボタン（⊞）から選択 |
| 分割サイズ変更 | ペイン間の仕切りをドラッグ |

プロファイルと一致するタブはプロファイル名で表示されます。

---

### セッションの再接続

WebSocket が切断されても **15 分間**はサーバー側で SSH セッションが保持されます。
ブラウザをリロードするか再度アクセスすると自動で再接続されます。

---

### セッションの共有（閲覧専用）

ターミナル右上の **Share** ボタンを押すと読み取り専用の共有 URL（`?share=<token>`）が発行され、クリップボードにコピーされます。
この URL を開いた相手は同じセッションをリアルタイムに閲覧できますが、入力はできません。
**Stop sharing** で共有を失効させると、ビューアの接続は切断されます。

---

### 接続ログ・録画

接続フォームのナビゲーションメニューから **Logs** を開くと、過去の接続履歴（接続/切断時刻、失敗時のエラー）を確認できます。
`DB_PATH` を設定している場合、ログは SQLite に永続化され再起動後も残ります。

`RECORDING_ENABLED` を有効にして接続したセッションは asciinema 形式で録画され、ログ画面の各エントリから再生できます。

同じメニューの **Sessions** からは現在アクティブなセッション一覧を表示し、任意のセッションを強制終了できます。

---

### カラーテーマ

ターミナル右上のテーマセレクタから配色を切り替えられます（Tokyo Night / Dracula / Solarized Dark / One Dark）。
選択したテーマは localStorage に保存され、次回起動時に引き継がれます。

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

フォントサイズは変更後も localStorage に保持され、次回起動時に引き継がれます。

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

| 変数名 | 必須 | デフォルト | 説明 |
|--------|------|-----------|------|
| `VAULT_ADDR` | ✅ | — | Vault サーバーのアドレス (例: `http://127.0.0.1:8200`) |
| `VAULT_TOKEN` | ✅ | — | Vault アクセストークン |
| `VAULT_SSH_ROLE` | ✅ | — | SSH 署名に使用するロール名 |
| `VAULT_SSH_MOUNT` | | `ssh` | Vault SSH Secrets Engine のマウントパス |
| `SERVER_PORT` | | `8080` | HTTP サーバーのリッスンポート |
| `GRACE_PERIOD` | | `15m` | WebSocket 切断後にセッションを保持する期間 |
| `SESSION_GC_INTERVAL` | | `1m` | 期限切れセッションの GC 実行間隔 |
| `CORS_ALLOWED_ORIGINS` | | `http://localhost:5173` | API アクセスを許可する CORS オリジン（カンマ区切り） |
| `KNOWN_HOSTS_PATH` | | — | ホスト鍵検証に使う known_hosts ファイルのパス。未設定時はホスト鍵検証を行わない |
| `DB_PATH` | | — | 接続ログを永続化する SQLite ファイルのパス。未設定時はメモリストアを使用 |
| `RECORDING_ENABLED` | | — | 値が設定されているとセッション録画を有効化 |
| `RECORDING_DIR` | | `./recordings` | `.cast` 録画ファイルの保存先ディレクトリ |

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

> バックエンドは `localhost:8080` で起動している必要があります。
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

### カバレッジ（現状）

| パッケージ | カバレッジ |
|-----------|-----------|
| `internal/vault` | 89.5% |
| `internal/recording` | 81.0% |
| `internal/config` | 80.6% |
| `pkg/token` | 75.0% |
| `internal/tunnel` | 69.7% |
| `internal/session` | 62.9% |
| `internal/connlog` | 61.5% |
| `internal/api` | 53.0% |
| `internal/sshconn` | 51.6% |

> ライブ WebSocket 接続を必要とする経路（`readPump` / `writePump` など）は静的テストでは未カバーです。

---

## API

### `POST /api/connect`

SSH 接続を確立してセッションを作成します。

**リクエスト**

```json
// Vault 証明書認証（デフォルト）
{ "host": "192.168.1.10", "port": 22, "user": "ubuntu", "auth_type": "vault" }

// パスワード認証
{ "host": "192.168.1.10", "port": 22, "user": "admin", "auth_type": "password", "password": "..." }

// 公開鍵認証
{ "host": "192.168.1.10", "port": 22, "user": "ubuntu", "auth_type": "pubkey", "private_key": "-----BEGIN OPENSSH PRIVATE KEY-----\n..." }

// ProxyJump（踏み台経由）— jump_* フィールドを追加（任意）
{
  "host": "10.0.0.5", "port": 22, "user": "ubuntu", "auth_type": "vault",
  "jump_host": "bastion.example.com", "jump_port": 22, "jump_user": "ubuntu", "jump_auth_type": "vault"
}
```

**レスポンス (201)**

```json
{
  "session_token": "a3f9...",
  "expires_at": "2024-01-01T00:15:00Z",
  "message": "SSH session established to 192.168.1.10:22"
}
```

### `GET /ws?token=<session_token>`

WebSocket にアップグレードして双方向ターミナルストリームを開きます。

- **Binary frame** — ターミナルの入出力データ
- **Text frame** — 制御メッセージ (JSON)

  ```json
  { "type": "ping" }
  { "type": "resize", "cols": 120, "rows": 40 }
  ```

### `GET /ws?share=<share_token>`

共有トークンで読み取り専用ビューアとして接続します。入力は無視され、出力のみがストリームされます。

### `GET /api/sessions`

アクティブなセッションの一覧を返します（管理 UI 用）。

### `DELETE /api/sessions/{token}`

指定したセッションを強制終了します（`204 No Content`）。

### `POST /api/sessions/{token}/share`

セッションに対する読み取り専用の共有トークンを発行します。

```json
// レスポンス (201)
{ "share_token": "…", "url": "http://<host>/?share=…", "expires_at": "2024-01-01T00:15:00Z" }
```

### `DELETE /api/sessions/{token}/share/{shareToken}`

共有トークンを失効させます（`204 No Content`）。

### `GET /api/logs`

接続ログ（接続/切断時刻、エラー、録画パスの有無）を新しい順に返します。

### `GET /api/recordings/{id}`

接続ログ ID に対応する asciinema 録画（`application/x-asciicast`）を配信します。

### `GET /healthz`

ライブネスプローブ（`{"status":"ok"}`）。

---

## ライセンス

MIT
