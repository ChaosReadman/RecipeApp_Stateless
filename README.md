# NutrientDB - 食品栄養素データベース

+ ２つのプロジェクトから成り立ちます
+ 食品成分表SQLiteに変換し検索文字列に対応した全栄養情報を返します。（こちらは適当なサーバにサービスを立てる）
+ 上記サービスから材料を選択し、献立を作成、朝食、昼食、夕食でそれら献立を組み合わせ日付時刻を登録する（登録先は自分のGoogleアカウントのスプレッドシート）

## 1. 環境構築

```bash
cd RecipeApp_Stateless/nutrient-api
# Goモジュールの初期化と依存関係のインストール
go mod init nutrient-api
go mod tidy
```
## 2. データベースの準備
XMLデータからSQLiteデータベース（`data/nutrient.db`）を生成します。

```bash
cd data
python3 data/xml_to_sqlite.py
```

## 3.献立登録アプリの開発

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
echo 'export GOPATH=$(go env GOPATH)' >> ~/.zshrc
echo 'export PATH=$PATH:$GOPATH/bin' >> ~/.zshrc
source ~/.zshrc
```


## 4. 開発サーバーの起動（ホットリロード有効）

実行には環境変数の設定が必要です。

プロジェクトルートに `.env` ファイルを作成してください：
※ 値はチームの共有パスワードマネージャーを参照してください。

```env
GOOGLE_CLIENT_ID="あなたのクライアントID"
GOOGLE_CLIENT_SECRET="あなたのクライアントシークレット"
GOOGLE_REDIRECT_URL="http://localhost:3000/auth/callback"
```

その後、以下のコマンドで起動します：

```bash
air
```
