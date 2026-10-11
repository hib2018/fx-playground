# Workbench — Go + TypeScript + fx terminal

ローカル専用のモックアップです。タスクボードとコマンドのデモはAPIキーなしで使えます。
「fxを起動」は **本物の `createFxTerminal()`** を呼び、ブラウザ内で `fx-term.wasm` を実行します。
AI応答だけはAI Gatewayへの接続が必要です。CLIのログインは引き継ぎません。

## 必要なもの

- Go 1.22以降（`server/go.mod`のバージョンも参照）
- Node.js 22.12以降（24推奨）、npm
- JSPIに対応したブラウザ（対応するChrome / Edgeなど）。画面に検出結果が表示されます

## 起動

リポジトリのルートから、ターミナルを2つ開きます。

```sh
# ターミナル1: APIサーバー
cd terminal-mockup/server
go run .
```

```sh
# ターミナル2: フロントエンド
cd terminal-mockup/web
npm ci
npm run dev
```

http://127.0.0.1:5173 を開いてください。

1. タスクをクリックすると Todo → In progress → Done → Todo と変わります。
2. 「モックを試す」で `pwd`、`ls`、`go test ./...` の固定結果を表示します。実機のテストではありません。
3. 「fxを起動」でWASMターミナルを起動し、`/help`や`/status`を試せます。
4. APIキーなしではモデルへのリクエストは503になります。画面にも未接続と表示されます。
5. 「停止」でruntimeを終了し、再起動できます。

## AI応答を有効にする（課金対象）

**Goサーバー側だけ**に環境変数 `AI_GATEWAY_API_KEY` を設定して起動し直します。
キーをフロントエンドのコードや `VITE_*` 環境変数に入れないでください。

```sh
cd terminal-mockup/server
# AI_GATEWAY_API_KEYを安全に環境変数へ設定してから実行
go run .
```

画面を再読み込みしてfxを起動します。ブラウザの認証値はダミーで、Goが実際のBearerキーに置き換えます。
本モックはGateway接続に限定しており、Codex/GrokのOAuthアダプタは実装していません。
設定だけlocalStorageに保存します。会話・入力履歴・OAuthトークンの永続化は未実装です。

## 構成

```text
terminal-mockup/
├── server/           Go標準ライブラリのHTTP API・プロキシ・テスト
└── web/
    ├── src/main.ts   ダッシュボードとcreateFxTerminalの接続
    ├── src/style.css レスポンシブUI
    ├── src/libfx-browser.d.ts 使用するAPIの型（libfx 0.0.13は型宣言なし）
    └── tests/        Playwrightによるブラウザ確認
```

Viteは `/api` をGoへ転送します。fxのHTTP通信も `/api/gateway` 経由です。
GoはGatewayのモデル一覧GETと推論POSTだけを許可し、クライアントのキーやCookieを上流に転送しません。
リクエスト本文は1 MiBまで、上流通信は最大2分。キャンセルとストリーミングに対応します。

`workspace`アダプタは `/api/workspace/exec` に接続します。
対応コマンドは `pwd`、`ls`、`cat README.md`、`go test ./...`、`git status` のみ。
すべて固定のモック結果で、OSコマンドは一切実行しません。
タスクはサーバーのメモリ内のみ保存され、再起動で初期化されます。

## ビルドしてGoだけで配信

```sh
cd terminal-mockup/web
npm ci
npm run build
cd ../server
go run . -static ../web/dist
```

http://127.0.0.1:8080 を開きます。WASMアセットもビルドに含まれます。

## 検証

```sh
cd terminal-mockup/server
go test -race ./...
go vet ./...
```

```sh
cd terminal-mockup/web
npm run build
npx playwright install chromium
npm run test:e2e
```

macOSのインストール済みChromeを使う場合：

```sh
PLAYWRIGHT_CHROME_PATH='/Applications/Google Chrome.app/Contents/MacOS/Google Chrome' npm run test:e2e
```

ブラウザテストはキーを空にしてGoを起動し、モデル推論を行わずにボード操作、モック実行、WASM起動、`/help`、停止、モバイル幅を確認します。8080ポートを空けて実行してください。

## 制限・安全性

- ローカルデモ用。サーバーは `127.0.0.1:8080` に固定しています。
- アプリ認証やユーザーごとのレート制限はありません。公開サーバーにそのまま載せないでください。
- ブラウザからのOriginはローカルViteまたはローカルGo配信に制限します。
- 会話・モデルへの実接続は課金されます。E2Eテストでは推論リクエストがゼロであることを検証します。
- `allow-sandboxed`は安全な隔離環境を自動作成するものではありません。本モックではGoの固定コマンド許可リストで制限しています。
- 実ファイル編集、対話プロセス、TTY、OAuthログインは未実装です。

公式資料: https://fx.sh/docs/lib/terminal / https://fx.sh/docs/lib/api
