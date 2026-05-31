# Conduit — Web SSH Terminal

[![CI](https://github.com/nagayon-935/Conduit/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/nagayon-935/Conduit/actions/workflows/ci.yml)
[![Coverage Status](https://coveralls.io/repos/github/nagayon-935/conduit/badge.svg?branch=main)](https://coveralls.io/github/nagayon-935/conduit?branch=main)
[![Go Report Card](https://goreportcard.com/badge/github.com/nagayon-935/conduit)](https://goreportcard.com/report/github.com/nagayon-935/conduit)
[![Go Version](https://img.shields.io/badge/Go-1.22-00ADD8?logo=go&logoColor=white)](https://go.dev/doc/go1.22)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

> ⚠️ **開発中 (Work in Progress)**
> このプロジェクトは現在開発中であり、実環境での動作確認は行っていません。
> 本番環境での使用は推奨しません。

ブラウザから SSH に接続できる Web ターミナルアプリケーションです。
HashiCorp Vault が発行する短命 SSH 証明書（TTL=5分）で認証し、WebSocket 経由でリアルタイムにターミナルを操作できます。

---

## アーキテクチャ

```text
Browser (xterm.js)
    │  WebSocket (binary frames)
    ▼
Go HTTP Server
    ├─ POST /api/connect   → Vault で証明書発行 → SSH 接続確立 → セッション生成
    └─ GET  /ws            → WebSocket ↔ SSH ストリームブリッジ
         │
         ▼
    Target SSH Server  (証明書認証)
```

### 主要な設計ポイント

| 機能 | 詳細 |
|------|------|
| **短命 SSH 証明書** | Vault SSH Secrets Engine で TTL=5分の証明書を発行。秘密鍵はメモリ上のみに保持しディスクに書かない |
| **グレース期間再接続** | WebSocket 切断後 15 分間は SSH セッションを保持。同じトークンで再接続すると続きから操作できる |
| **バックプレッシャー** | SSH → クライアント方向のチャンネルが詰まった場合、50ms 待って送れなければドロップ。ゴルーチンのフリーズを防ぐ |

---

## 技術スタック

### バックエンド

- **Go 1.22**
- `golang.org/x/crypto/ssh` — SSH クライアント・証明書認証
- `github.com/gorilla/websocket` — WebSocket サーバー
- HashiCorp Vault HTTP API — SSH 証明書署名

### フロントエンド

- **React 18 + TypeScript**
- `@xterm/xterm` — ターミナルエミュレータ（WebGL レンダラー）
- `@xterm/addon-fit` — ウィンドウサイズ自動追従
- `@xterm/addon-webgl` — GPU アクセラレーション描画
- Vite 5 — ビルドツール・開発サーバー

---

## ディレクトリ構成

```text
.
├── cmd/server/          # エントリポイント (main.go)
├── internal/
│   ├── api/             # HTTP ハンドラー (connect, terminal, middleware)
│   ├── config/          # 環境変数設定
│   ├── session/         # セッション状態管理・GC
│   ├── sshconn/         # 鍵生成・SSH ダイアル・証明書サイナー
│   ├── tunnel/          # WebSocket↔SSH ポンプ・PTY リサイズ
│   └── vault/           # Vault クライアント
├── pkg/token/           # セッショントークン生成
├── tests/               # E2E 統合テスト
└── frontend/            # React フロントエンド
    └── src/
        ├── api/         # REST クライアント
        ├── components/  # ConnectForm, Terminal
        ├── hooks/       # useTerminal, useWebSocket
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

> パスワード・秘密鍵はブラウザの localStorage に保存されません。

#### 複数ホストへの同時接続

**+ Add host** ボタンで接続先を追加すると、**Connect All** で全ホストへ並列接続してスプリット表示できます。

---

### プロファイル

よく使う接続先をプロファイルとして保存できます。

- **保存**: フォーム入力後、**+ Save as Profile** からプロファイル名を入力して保存
- **読み込み**: Profiles リストのプロファイルをクリックすると Host・Port・User・認証方式が自動入力
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

## ローカル開発セットアップ

### 前提条件

- Go 1.22+
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
| `internal/config` | 100% |
| `internal/vault` | 89.5% |
| `internal/session` | 86.2% |
| `internal/api` | 76.5% |
| `pkg/token` | 75.0% |
| `internal/sshconn` | 71.8% |
| `internal/tunnel` | 33.7% ※ |

> ※ `readPump` / `writePump` / `handleControlMessage` はライブ WebSocket 接続が必要なため静的テストでは未カバー

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
```

**レスポンス (201)**

```json
{
  "session_token": "a3f9...",
  "expires_at": "2024-01-01T00:15:00Z",
  "message": "session created"
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

---

## ライセンス

MIT
