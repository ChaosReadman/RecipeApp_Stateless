package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/template/html/v2"
)

// 全データをそのまま保持する型定義 (APIのレスポンス形式)
type FoodMap map[string]any

// 一時的なメモリ保存用（本来はセッションやDBで管理）
var (
	mu                  sync.Mutex
	selectedIngredients []FoodMap
)

// 合計栄養素を計算する構造体
type NutrientSummary struct {
	Energy float64 `json:"energy"`
	Prot   float64 `json:"protein"`
	Fat    float64 `json:"fat"`
	Cho    float64 `json:"carbohydrate"`
	K      float64 `json:"potassium"`
	Ca     float64 `json:"calcium"`
	Fe     float64 `json:"iron"`
	Zn     float64 `json:"zinc"`
	VitA   float64 `json:"vitamin_a"`
	VitB   float64 `json:"vitamin_b"` // B1を代表値とする
	VitC   float64 `json:"vitamin_c"`
	Fiber  float64 `json:"fiber"`
	Salt   float64 `json:"salt"`
}

var myIngredients = []FoodMap{
	{"num_id": "01001", "name": "アマランサス　玄穀", "weight": 100.0},
	{"num_id": "01002", "name": "あわ　精白粒", "weight": 100.0},
}

// スレッドセーフに材料リストを取得するヘルパー
func getIngredients() []FoodMap {
	mu.Lock()
	defer mu.Unlock()
	res := make([]FoodMap, len(selectedIngredients))
	copy(res, selectedIngredients)
	return res
}

func main() {
	// テンプレートエンジンの初期化
	engine := html.New("./views", ".html")

	app := fiber.New(fiber.Config{
		Views:     engine,
		Immutable: true,
	})

	// メイン画面（検索ロジック）
	app.Get("/", func(c *fiber.Ctx) error {
		query := c.Query("q")
		recipeQuery := c.Query("rq")
		var foods []FoodMap

		ingredients := getIngredients()
		summary := NutrientSummary{}

		// 合計栄養素の計算
		for _, ing := range ingredients {
			weight, _ := ing["weight"].(float64)
			wRatio := weight / 100.0

			// ヘルパー関数: 複数のキーから値を加算。型が何であっても数値として抽出を試みる
			addVal := func(target *float64, keys ...string) {
				for _, k := range keys {
					val, ok := ing[strings.ToLower(k)]
					if !ok || val == nil {
						continue
					}

					var fVal float64
					var err error

					switch v := val.(type) {
					case float64:
						fVal = v
					case int64:
						fVal = float64(v)
					case string:
						// "(0)" や "Tr" などの成分表特有の表記を考慮
						cleaned := strings.TrimSpace(v)
						if cleaned == "-" || cleaned == "Tr" || cleaned == "(0)" {
							fVal = 0
						} else {
							fVal, err = strconv.ParseFloat(cleaned, 64)
						}
					}

					if err == nil {
						*target += fVal * wRatio
						return // 一つ見つかったら終了
					}
				}
			}

			// 日本食品標準成分表のあらゆるキーに対応
			addVal(&summary.Energy, "enerc_kcal", "enerc")
			addVal(&summary.Prot, "prot_", "prot", "protein")
			addVal(&summary.Fat, "fat_", "fat")
			addVal(&summary.Cho, "choavl", "chocdf", "cho")
			addVal(&summary.K, "k", "potassium")
			addVal(&summary.Ca, "ca", "calcium")
			addVal(&summary.Fe, "fe", "iron")
			addVal(&summary.Zn, "zn", "zinc")
			addVal(&summary.VitA, "vita_rae", "vita")
			addVal(&summary.VitB, "thia", "vitb1")
			addVal(&summary.VitC, "vitc", "vitamin_c")
			addVal(&summary.Fiber, "fib_", "fibtg", "fiber")

			// 食塩相当量の計算（NACL_EQを優先し、なければナトリウムNAから換算）
			var salt float64
			addVal(&salt, "nacl_eq", "salt")
			if salt == 0 {
				var sodium float64
				addVal(&sodium, "na", "sodium")
				salt = sodium * 2.54 / 1000.0
			}
			summary.Salt += salt

			log.Printf("[CALC] Item: %v (%.1fg)\n"+
				"  Energy: %.1f, Prot: %.1f, Fat: %.1f, Cho: %.1f\n"+
				"  K: %.1f, Ca: %.1f, Fe: %.1f, Zn: %.1f\n"+
				"  VitA: %.1f, VitB: %.2f, VitC: %.1f, Fiber: %.1f, Salt: %.2f",
				ing["name"], weight, summary.Energy, summary.Prot, summary.Fat, summary.Cho,
				summary.K, summary.Ca, summary.Fe, summary.Zn, summary.VitA, summary.VitB, summary.VitC, summary.Fiber, summary.Salt)
		}

		if query != "" {
			// nutrient-api へのリクエスト
			apiUrl := fmt.Sprintf("http://127.0.0.1:8080/api/foods/search?q=%s", url.QueryEscape(query))
			resp, err := http.Get(apiUrl)
			if err == nil {
				defer resp.Body.Close()
				if decodeErr := json.NewDecoder(resp.Body).Decode(&foods); decodeErr != nil {
					log.Printf("[API ERROR] Decode failed: %v", decodeErr)
				} else if len(foods) > 0 {
					// 最初の1件の全キーを表示して、正しいIDカラムを特定する
					keys := make([]string, 0, len(foods[0]))
					for k := range foods[0] {
						keys = append(keys, k)
					}
					log.Printf("[DEBUG] API Response Keys: %v", keys)
				}
			}
		}

		// テンプレートのレンダリング
		return c.Render("index", fiber.Map{
			"Title":         "NutriTrack",
			"Query":         query,
			"RecipeQuery":   recipeQuery,
			"Foods":         foods,
			"Recipes":       []string{},
			"Ingredients":   ingredients,
			"Summary":       summary,
			"MyIngredients": myIngredients,
		})
	})

	// 食材をリストに追加
	app.Post("/ingredients/add", func(c *fiber.Ctx) error {
		id := c.FormValue("id")
		name := c.FormValue("name")
		weight, _ := strconv.ParseFloat(c.FormValue("weight"), 64)
		if weight == 0 {
			weight = 100
		}

		if id != "" {
			log.Printf("[ACTION: ADD] ID: %s, Name: %s", id, name)
			mu.Lock()
			// すでにリストに存在する食材かチェック
			found := false
			for i := range selectedIngredients {
				// どんな型でも文字列にして比較（末尾の.0も除去）
				itemIDStr := strings.Split(fmt.Sprintf("%v", selectedIngredients[i]["num_id"]), ".")[0]
				if itemIDStr == id {
					currentW, _ := selectedIngredients[i]["weight"].(float64)
					selectedIngredients[i]["weight"] = currentW + weight
					log.Printf("[ADD] Found existing item. Updated weight to %.1f", selectedIngredients[i]["weight"])
					found = true
					break
				}
			}
			mu.Unlock()

			// すでに見つかった場合は重量更新のみで終了
			if found {
				return c.Redirect("/")
			}

			// 詳細な栄養素を取得するためにAPIを叩く
			var details []FoodMap
			apiUrl := fmt.Sprintf("http://127.0.0.1:8080/api/foods/search?id=%s", url.QueryEscape(id))
			log.Printf("[API FETCH] URL: %s", apiUrl)
			resp, err := http.Get(apiUrl)
			if err == nil {
				defer resp.Body.Close()
				if decodeErr := json.NewDecoder(resp.Body).Decode(&details); decodeErr != nil {
					log.Printf("[API ERROR] Decode failed: %v", decodeErr)
				}
			}

			var nutrientData FoodMap
			if len(details) > 0 {
				nutrientData = details[0] // APIから取得した全データをそのまま保持
				nutrientData["weight"] = weight
			} else {
				// 万が一APIにデータがない場合でも最低限の情報で追加
				nutrientData = FoodMap{"num_id": id, "name": name, "weight": weight}
			}

			mu.Lock()
			selectedIngredients = append(selectedIngredients, nutrientData)
			mu.Unlock()
		}
		return c.Redirect("/")
	})

	// 食材の重量を更新
	app.Post("/ingredients/update", func(c *fiber.Ctx) error {
		id := c.FormValue("id")
		weight, _ := strconv.ParseFloat(c.FormValue("weight"), 64)
		log.Printf("[ACTION: UPDATE] ID: %s, New Weight: %.1f", id, weight)
		mu.Lock()
		for i := range selectedIngredients {
			if fmt.Sprintf("%v", selectedIngredients[i]["num_id"]) == id {
				selectedIngredients[i]["weight"] = weight
				break
			}
		}
		mu.Unlock()
		return c.Redirect("/")
	})

	// 食材をリストから削除
	app.Post("/ingredients/remove", func(c *fiber.Ctx) error {
		id := c.FormValue("id")
		log.Printf("[ACTION: REMOVE] ID: %s", id)
		mu.Lock()
		defer mu.Unlock()
		var newList []FoodMap
		for _, item := range selectedIngredients {
			// IDを文字列に正規化して比較
			itemIDStr := strings.TrimSuffix(fmt.Sprintf("%v", item["num_id"]), ".0")
			if itemIDStr != id {
				newList = append(newList, item)
			}
		}
		selectedIngredients = newList
		return c.Redirect("/")
	})

	// 食品詳細画面（仮）
	app.Get("/food/:id", func(c *fiber.Ctx) error {
		return c.SendString(fmt.Sprintf("食品ID: %s の詳細画面（未実装）", c.Params("id")))
	})

	// 静的ファイルの配信設定（ルート定義の後に配置）
	app.Static("/", "./public")

	log.Fatal(app.Listen(":3000"))
}
