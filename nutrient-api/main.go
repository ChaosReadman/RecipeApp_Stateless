package main

import (
	"database/sql"
	"log"
	"os"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/joho/godotenv"
	_ "github.com/mattn/go-sqlite3"
)

func main() {
	// .envファイルを読み込む（存在しなくてもエラーにはしない）
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using default settings")
	}

	app := fiber.New()

	// CORSミドルウェアを追加 (フロントエンドからのアクセスを許可)
	app.Use(cors.New())

	// データベース接続 (読み取り専用で開くのが安全)
	db, err := sql.Open("sqlite3", "./data/nutrient.db")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	// 検索APIエンドポイント
	app.Get("/api/foods/search", func(c *fiber.Ctx) error {
		searchTerm := c.Query("q")
		searchID := c.Query("id")

		log.Printf("[API REQUEST] Query: '%s', ID: '%s'", searchTerm, searchID)

		// 「全データを返す」ため SELECT * を使用。検索語があれば絞り込み、なければ全件取得
		sqlQuery := "SELECT * FROM foods"
		var args []interface{}

		if searchID != "" {
			sqlQuery += " WHERE num_id = ?"
			args = append(args, searchID)
		} else if searchTerm != "" {
			sqlQuery += " WHERE name LIKE ?"
			args = append(args, "%"+searchTerm+"%")
		}

		rows, err := db.Query(sqlQuery, args...)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Failed to query database",
			})
		}
		defer rows.Close()

		// カラム名を動的に取得
		cols, err := rows.Columns()
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).SendString(err.Error())
		}
		log.Printf("[API DEBUG] Database Columns: %v", cols)

		var results []fiber.Map
		for rows.Next() {
			// 動的なスキャンのためのバッファ準備
			columns := make([]interface{}, len(cols))
			columnPointers := make([]interface{}, len(cols))
			for i := range columns {
				columnPointers[i] = &columns[i]
			}

			if err := rows.Scan(columnPointers...); err != nil {
				continue
			}

			row := make(fiber.Map)
			for i, colName := range cols {
				val := columns[i]
				// カラム名を小文字に統一してJSONキーの不一致を防ぐ
				key := strings.ToLower(colName)
				// SQLiteの文字列データ ([]byte) を文字列に変換
				if b, ok := val.([]byte); ok {
					row[key] = string(b)
				} else {
					row[key] = val
				}
			}
			results = append(results, row)
		}
		log.Printf("[API RESPONSE] Returning %d items", len(results))
		return c.JSON(results)
	})

	// ポート番号を環境変数から取得（デフォルトは8080）
	port := os.Getenv("API_PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Nutrient API starting on port %s", port)
	log.Fatal(app.Listen(":" + port))
}
