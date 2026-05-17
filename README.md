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
