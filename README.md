# NutrientDB - 食品栄養素データベース

+ ２つのプロジェクトから成り立ちます
+ 食品成分表SQLiteに変換し検索文字列に対応した全栄養情報を返します。（こちらは適当なサーバにサービスを立てる）
+ 上記サービスから材料を選択し、献立を作成、朝食、昼食、夕食でそれら献立を組み合わせ日付時刻を登録する（登録先は自分のGoogleアカウントのスプレッドシート）

## 1. nutrient-apiの開発

```bash
cd RecipeApp_Stateless/nutrient-api
# Goモジュールの初期化と依存関係のインストール
go mod init nutrient-api
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


## 4. NutriTrackのの起動（ホットリロード有効）

```bash
air
```
