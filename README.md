# NutriTrack - 栄養管理 & 献立作成システム

このプロジェクトは、以下の2つのサービスで構成されるフルスタックアプリケーションです。

1.  **Nutrient API** (Port 8080): 日本食品標準成分表のSQLiteデータベースから栄養情報を検索・提供するバックエンド。
2.  **NutriTrack** (Port 3000): 献立の作成、栄養素の計算、およびGoogleスプレッドシートへの登録を行うフロントエンド/メインロジック。

---

## 1. 事前準備 (環境構築)

### 依存ツールのインストール
開発効率向上のためのホットリロードツール `air` をインストールします。

```bash
go install github.com/air-verse/air@latest
```
※実行パスが通っていない場合は、`~/.zprofile` 等に `$GOPATH/bin` を追加してください。

---

## 2. Nutrient API (バックエンド) のセットアップ

```bash
cd RecipeApp_Stateless/nutrient-api
go mod tidy
```
## 2. 栄養素データベースの準備
XMLデータからSQLiteデータベース（`data/nutrient.db`）を生成します。

```bash
cd data
python3 data/xml_to_sqlite.py
```

## 3.NutriTrack（献立登録アプリ）の開発

```bash
cd RecipeApp_Stateless/NutriTrack
go mod init NutriTrack
go get github.com/gofiber/fiber/v2
go get github.com/gofiber/template/html/v2
go get github.com/mattn/go-sqlite3
go get github.com/joho/godotenv
go get golang.org/x/oauth2
go get golang.org/x/oauth2/google
go mod tidy

### Google OAuth2 環境変数の設定
Googleログイン機能を有効にするには、Google Cloud Console で取得したクライアントIDとクライアントシークレットを環境変数として設定する必要があります。
`.env` ファイルを作成し、以下の形式で記述してください。

```
GOOGLE_CLIENT_ID="あなたのGoogleクライアントID"
GOOGLE_CLIENT_SECRET="あなたのGoogleクライアントシークレット"
```
`main.go` は `godotenv` を使用してこれらの環境変数を読み込みます。
```

# ホットリロードツール (Air) のインストール
```bash
go get -u github.com/air-verse/air
go install github.com/air-verse/air@latest
echo 'export GOPATH=$(go env GOPATH)' >> ~/.zprofile
echo 'export PATH=$PATH:$GOPATH/bin' >> ~/.zprofile
source ~/.zprofile
```


## 4. NutriTrackの起動（ホットリロード有効）

```bash
air
```

## 5. デバッグ方法 (VS Code)

本プロジェクトには VS Code 用の `launch.json` が構成されており、バックエンド（Nutrient API）とメインアプリ（NutriTrack）を同時に起動してデバッグできる **"Full Stack Debug"** モードが用意されています。

1. VS Code の **[実行とデバッグ]** ビュー（`Ctrl+Shift+D`）を開きます。
2. プロファイル選択メニューから **「Full Stack Debug」** を選択します。
3. **F5** キーを押してデバッグを開始します。

これにより、API（ポート 8080）と Webアプリ（ポート 3000）の両方のプロセスに対して同時にブレークポイントを設定し、ステップ実行を行うことが可能です。

