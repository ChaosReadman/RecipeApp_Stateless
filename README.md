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

パスが通っていない場合は、`~/.zprofile` 等に以下を追加してください：

```bash
echo 'export GOPATH=$(go env GOPATH)' >> ~/.zprofile
echo 'export PATH=$PATH:$GOPATH/bin' >> ~/.zprofile
source ~/.zprofile
```

---

## 2. Nutrient API (バックエンド) のセットアップ

### 2-1. Go モジュールの初期化

```bash
cd RecipeApp_Stateless/nutrient-api
go mod tidy
```

### 2-2. 栄養素データベースの準備

XMLデータからSQLiteデータベース（`nutrient-api/data/nutrient.db`）を生成します。

```bash
cd nutrient-api/data
python3 xml_to_sqlite.py
```

---

## 3. NutriTrack（献立登録アプリ）のセットアップ

```bash
cd RecipeApp_Stateless/NutriTrack
go mod init NutriTrack
go get github.com/gofiber/fiber/v2
go get github.com/gofiber/template/html/v2
go get github.com/mattn/go-sqlite3
go get github.com/joho/godotenv
go get golang.org/x/oauth2
go get golang.org/x/oauth2/google
go get google.golang.org/api/sheets/v4
go mod tidy
```

### 3-1. Google OAuth2 環境変数の設定

Googleログイン機能を有効にするには、Google Cloud Console で取得したクライアントIDとクライアントシークレットを環境変数として設定してください。

プロジェクトルートに `.env` ファイルを作成し、以下の形式で記述してください：

```env
GOOGLE_CLIENT_ID="あなたのGoogleクライアントID"
GOOGLE_CLIENT_SECRET="あなたのGoogleクライアントシークレット"
API_HOST="127.0.0.1"
API_PORT="8080"
APP_HOST="127.0.0.1"
APP_PORT="3000"
ENVIRONMENT="development"
```

`main.go` は `godotenv` を使用してこれらの環境変数を読み込みます。

**注意**: `.env` ファイルは `.gitignore` に含まれています。コミットされないため、共同開発者用に `.env.example` テンプレートを作成することをお勧めします。

### 3-2. .env.example テンプレート作成（推奨）

```bash
cp .env .env.example
# .env.example の認証情報を削除またはダミー値に置き換える
```

---

## 4. アプリケーションの起動

### 4-1. Nutrient API (ポート 8080) の起動

```bash
cd RecipeApp_Stateless/nutrient-api
air
```

### 4-2. NutriTrack (ポート 3000) の起動（別ターミナル）

```bash
cd RecipeApp_Stateless/NutriTrack
air
```

両方のサービスが起動したら、ブラウザで `http://127.0.0.1:3000` にアクセスしてください。

---

## 5. デバッグ方法 (VS Code)

本プロジェクトには VS Code 用の `launch.json` が構成されており、バックエンド（Nutrient API）とメインアプリ（NutriTrack）を同時に起動してデバッグできます。

1. VS Code の **[実行とデバッグ]** ビュー（`Ctrl+Shift+D`）を開きます。
2. プロファイル選択メニューから **「Full Stack Debug」** を選択します。
3. **F5** キーを押してデバッグを開始します。

これにより、API（ポート 8080）と Webアプリ（ポート 3000）の両方のプロセスに対して同時にブレークポイントを設定し、ステップ実行を行うことができます。

---

## 6. 環境変数一覧

| 環境変数 | 説明 | 例 |
|---------|------|-----|
| `GOOGLE_CLIENT_ID` | Google OAuth2 クライアントID | `xxxxx.apps.googleusercontent.com` |
| `GOOGLE_CLIENT_SECRET` | Google OAuth2 クライアントシークレット | `xxxxx` |
| `API_HOST` | Nutrient API のホストアドレス | `127.0.0.1` (開発), `api.example.com` (本番) |
| `API_PORT` | Nutrient API のポート番号 | `8080` |
| `APP_HOST` | NutriTrack アプリのホストアドレス | `127.0.0.1` (開発), `app.example.com` (本番) |
| `APP_PORT` | NutriTrack アプリのポート番号 | `3000` |
| `ENVIRONMENT` | 実行環境 | `development` または `production` |

---

## トラブルシューティング

### air コマンドが見つからない
```bash
# GoPath の bin ディレクトリが PATH に含まれていることを確認
echo $PATH | grep $(go env GOPATH)/bin

# 見つからない場合はシェル設定ファイルに追加
echo 'export PATH=$PATH:$(go env GOPATH)/bin' >> ~/.bashrc
source ~/.bashrc
```

### データベースファイルが見つからない
```bash
# nutrient-api/data ディレクトリが存在し、nutrient.db が生成されているか確認
ls -la RecipeApp_Stateless/nutrient-api/data/nutrient.db
```

### API が接続できない
- Nutrient API (ポート 8080) が起動しているか確認
- ファイアウォール設定をチェック
- `.env` の `API_HOST` と `API_PORT` が正しいか確認

### OAuth コールバックエラー
- Google Cloud Console で登録した OAuth リダイレクト URI が `http://127.0.0.1:3000/auth/callback` と完全一致しているか確認
- 本番環境では https を使用してください
