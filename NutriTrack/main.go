package main

import (
	"NutriTrack/handlers"
	"NutriTrack/services"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/session"
	"github.com/gofiber/template/html/v2"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

// 全データをそのまま保持する型定義 (APIのレスポンス形式)
type FoodMap map[string]any

// API通信用の共通クライアント（タイムアウトを設定）
var apiClient = &http.Client{
	Timeout: 5 * time.Second,
}

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

// セッションから材料リストを取得するヘルパー
func getIngredients(sess *session.Session) []FoodMap {
	var ingredients []FoodMap
	if raw := sess.Get("ingredients"); raw != nil {
		if err := json.Unmarshal([]byte(raw.(string)), &ingredients); err != nil {
			log.Printf("[SESSION ERROR] Failed to unmarshal ingredients: %v", err)
		}
	}
	return ingredients
}

func main() {

	// 2. セッションストアの初期化
	store := session.New(session.Config{
		Expiration:     24 * time.Hour,
		CookieHTTPOnly: true,
		CookieSecure:   false, // HTTPでのローカル開発時はfalseにする
		CookieSameSite: "Lax",
	})

	// 3. Google OAuth2の設定
	clientID := os.Getenv("GOOGLE_CLIENT_ID")
	clientSecret := os.Getenv("GOOGLE_CLIENT_SECRET")

	if clientID == "" || clientSecret == "" {
		log.Fatal("環境変数 GOOGLE_CLIENT_ID と GOOGLE_CLIENT_SECRET を設定してください")
	}

	conf := &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURL:  "http://127.0.0.1:3000/auth/callback", // ここが Console の設定と完全一致している必要があります
		Scopes: []string{
			"https://www.googleapis.com/auth/userinfo.email",
			"https://www.googleapis.com/auth/userinfo.profile",
			"https://www.googleapis.com/auth/spreadsheets", // スプレッドシート操作権限
			"https://www.googleapis.com/auth/drive.file",   // アプリが作成したファイルへのアクセス権限
		},
		Endpoint: google.Endpoint,
	}

	// 4. ハンドラの初期化
	authHandler := &handlers.AuthHandler{
		Store:       store,
		OAuthConfig: conf,
	}

	foodHandler := &handlers.FoodHandler{
		Store:       store,
		OAuthConfig: conf,
	}

	// テンプレートエンジンの初期化
	engine := html.New("./views", ".html")
	engine.AddFunc("json", func(v interface{}) string {
		a, _ := json.Marshal(v)
		return string(a)
	})
	engine.Reload(true) // 開発用

	app := fiber.New(fiber.Config{
		Views:     engine,
		Immutable: true,
	})
	app.Static("/", "./public")

	// メイン画面（検索ロジック）
	app.Get("/", func(c *fiber.Ctx) error {
		query := c.Query("q")
		recipeQuery := c.Query("rq")
		var foods []FoodMap

		// セッションからログインユーザー名を取得
		sess, _ := store.Get(c)
		userName := sess.Get("username")
		ingredients := getIngredients(sess)

		// Google Driveからレシピを取得
		recipes := []map[string]interface{}{}
		if rawToken := sess.Get("oauth_token"); rawToken != nil {
			var token oauth2.Token
			if err := json.Unmarshal([]byte(rawToken.(string)), &token); err == nil {
				// conf は main.go 内で定義されている oauth2.Config
				client := conf.Client(c.Context(), &token)
				data, err := services.FetchRecipes(c.Context(), client, "", recipeQuery)
				if err == nil {
					recipes = data
				}
			}
		}

		summary := NutrientSummary{}

		// 合計栄養素の計算
		for _, ing := range ingredients {
			weight, _ := ing["weight"].(float64)
			wRatio := weight / 100.0

			// ヘルパー関数: 複数のキーから値を加算。型が何であっても数値として抽出を試みる
			addVal := func(target *float64, keys ...string) {
				for _, k := range keys {
					val, ok := ing[k]
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
			apiUrl := fmt.Sprintf("http://127.0.0.1:8080/api/foods/search?q=%s", query)
			resp, err := apiClient.Get(apiUrl)
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
			"Title":       "NutriTrack",
			"Query":       query,
			"RecipeQuery": recipeQuery,
			"Foods":       foods,
			"Recipes":     recipes,
			"Ingredients": ingredients,
			"Summary":     summary,
			"User":        userName,
		}, "layout")
	})

	// --- 認証ルート ---
	authGroup := app.Group("/auth")
	app.Get("/login", authHandler.ShowLogin)
	authGroup.Get("/login", authHandler.Login)
	authGroup.Get("/callback", authHandler.Callback)
	app.Get("/logout", authHandler.Logout)

	// --- レシピ関連ルート ---
	app.Get("/recipe/new", foodHandler.NewRecipe)
	app.Post("/recipe/create", foodHandler.CreateRecipe)
	app.Get("/recipe/:id/edit", foodHandler.EditRecipe)
	app.Post("/recipe/:id/update", foodHandler.UpdateRecipe)

	// レシピ詳細（調理画面）
	app.Get("/recipe/:id", func(c *fiber.Ctx) error {
		id := c.Params("id")
		sess, _ := store.Get(c)

		summary := NutrientSummary{}
		var recipe map[string]interface{}
		if rawToken := sess.Get("oauth_token"); rawToken != nil {
			var token oauth2.Token
			if err := json.Unmarshal([]byte(rawToken.(string)), &token); err == nil {
				// 認証クライアントの作成
				client := conf.Client(c.Context(), &token)
				recipes, err := services.FetchRecipes(c.Context(), client, "", "")
				if err == nil {
					for _, r := range recipes {
						if fmt.Sprintf("%v", r["ID"]) == id {
							// 工程（Steps）のデータ形式をテンプレート用に正規化（古い形式の互換性維持）
							if rawSteps, ok := r["Steps"].([]interface{}); ok {
								normalized := make([]map[string]interface{}, 0, len(rawSteps))
								for i, s := range rawSteps {
									switch v := s.(type) {
									case string:
										// 古いデータ形式（[]string）を変換
										normalized = append(normalized, map[string]interface{}{
											"StepNumber":  i + 1,
											"Instruction": v,
										})
									case map[string]interface{}:
										// すでに新しい形式の場合はそのまま利用
										normalized = append(normalized, v)
									}
								}
								r["Steps"] = normalized
							}

							// ヘルパー関数: 複数のキーから値を加算（計算用）
							addVal := func(target *float64, ing map[string]interface{}, wRatio float64, keys ...string) {
								for _, k := range keys {
									val, ok := ing[k]
									if !ok || val == nil {
										continue
									}
									var fVal float64
									switch v := val.(type) {
									case float64:
										fVal = v
									case string:
										cleaned := strings.TrimSpace(v)
										if cleaned == "-" || cleaned == "Tr" || cleaned == "(0)" {
											fVal = 0
										} else {
											fVal, _ = strconv.ParseFloat(cleaned, 64)
										}
									}
									*target += fVal * wRatio
									return
								}
							}

							// 材料（Ingredients）のデータ形式をテンプレート用に正規化（Weight -> Quantity, Group -> GroupName）
							if rawIngs, ok := r["Ingredients"].([]interface{}); ok {
								normalizedIngs := make([]map[string]interface{}, 0, len(rawIngs))
								for _, ing := range rawIngs {
									if m, ok := ing.(map[string]interface{}); ok {
										// 栄養素の再計算のためにAPIから最新データを取得
										ingID := fmt.Sprintf("%v", m["ID"])
										apiUrl := fmt.Sprintf("http://127.0.0.1:8080/api/foods/search?id=%s", ingID)
										resp, err := apiClient.Get(apiUrl)
										if err == nil {
											var details []FoodMap
											if decodeErr := json.NewDecoder(resp.Body).Decode(&details); decodeErr == nil && len(details) > 0 {
												nutrientData := details[0]
												weight, _ := m["Quantity"].(float64)
												if weight == 0 {
													weight, _ = m["Weight"].(float64)
												}
												wRatio := weight / 100.0

												// 各栄養素の加算
												addVal(&summary.Energy, nutrientData, wRatio, "enerc_kcal", "enerc")
												addVal(&summary.Prot, nutrientData, wRatio, "prot_", "prot", "protein")
												addVal(&summary.Fat, nutrientData, wRatio, "fat_", "fat")
												addVal(&summary.Cho, nutrientData, wRatio, "choavl", "chocdf", "cho")
												addVal(&summary.K, nutrientData, wRatio, "k", "potassium")
												addVal(&summary.Ca, nutrientData, wRatio, "ca", "calcium")
												addVal(&summary.Fe, nutrientData, wRatio, "fe", "iron")
												addVal(&summary.Zn, nutrientData, wRatio, "zn", "zinc")
												addVal(&summary.VitA, nutrientData, wRatio, "vita_rae", "vita")
												addVal(&summary.VitB, nutrientData, wRatio, "thia", "vitb1")
												addVal(&summary.VitC, nutrientData, wRatio, "vitc", "vitamin_c")
												addVal(&summary.Fiber, nutrientData, wRatio, "fib_", "fibtg", "fiber")

												var salt float64
												addVal(&salt, nutrientData, wRatio, "nacl_eq", "salt")
												if salt == 0 {
													var sodium float64
													addVal(&sodium, nutrientData, wRatio, "na", "sodium")
													salt = sodium * 2.54 / 1000.0
												}
												summary.Salt += salt
											}
											resp.Body.Close()
										}

										// テンプレートが期待するキー名（Quantity, GroupName）に統一する
										if w, exists := m["Weight"]; exists {
											m["Quantity"] = w
										}
										if g, exists := m["Group"]; exists {
											m["GroupName"] = g
										}
										normalizedIngs = append(normalizedIngs, m)
									}
								}
								r["Ingredients"] = normalizedIngs
							}
							recipe = r
							break
						}
					}
				}
			}
		}

		if recipe == nil {
			return c.Redirect("/")
		}

		// レシピフローと食事記録フローを分離するため、詳細表示時にセッションをクリア（Dirty Session防止）
		sess.Delete("ingredients")
		_ = sess.Save()

		return c.Render("recipe_detail", fiber.Map{
			"Title":   fmt.Sprintf("%v", recipe["Title"]),
			"Recipe":  recipe,
			"User":    sess.Get("username"),
			"Summary": summary,
			"IsOwner": true,
		}, "layout")
	})

	// --- 食材・レシピ関連ルート (foodHandlerの例) ---
	// 食材をリストに追加
	app.Post("/ingredients/add", func(c *fiber.Ctx) error {
		sess, _ := store.Get(c)
		ingredients := getIngredients(sess)

		id := c.FormValue("id")
		name := c.FormValue("name")
		weight, _ := strconv.ParseFloat(c.FormValue("weight"), 64)
		if weight == 0 {
			weight = 100
		}

		if id != "" {
			log.Printf("[ACTION: ADD] ID: %s, Name: %s", id, name)
			// すでにリストに存在する食材かチェック
			found := false
			for i := range ingredients {
				// どんな型でも文字列にして比較（末尾の.0も除去）
				itemIDStr := strings.TrimSuffix(fmt.Sprintf("%v", ingredients[i]["num_id"]), ".0")
				if itemIDStr == id {
					currentW, _ := ingredients[i]["weight"].(float64)
					ingredients[i]["weight"] = currentW + weight
					found = true
					break
				}
			}

			// すでに見つかった場合は重量更新のみで終了
			if found {
				data, _ := json.Marshal(ingredients)
				sess.Set("ingredients", string(data))
				sess.Save()
				return c.Redirect("/")
			}

			// 詳細な栄養素を取得するためにAPIを叩く
			var details []FoodMap
			// APIの仕様(id=)に合わせてリクエスト
			apiUrl := fmt.Sprintf("http://127.0.0.1:8080/api/foods/search?id=%s", id)
			log.Printf("[API FETCH] URL: %s", apiUrl)
			resp, err := apiClient.Get(apiUrl)
			if err != nil {
				log.Printf("[API ERROR] Request failed: %v", err)
			} else if decodeErr := json.NewDecoder(resp.Body).Decode(&details); decodeErr != nil {
				log.Printf("[API ERROR] Decode failed: %v", decodeErr)
			}

			var nutrientData FoodMap
			if len(details) > 0 {
				nutrientData = details[0]
				nutrientData["weight"] = weight
			} else {
				// データが取得できなかった場合
				log.Printf("[WARN] No nutrient data found for ID: %s. Check if API is working correctly.", id)
				nutrientData = FoodMap{"num_id": id, "name": name, "weight": weight}
			}

			ingredients = append(ingredients, nutrientData)
			data, _ := json.Marshal(ingredients)
			sess.Set("ingredients", string(data))
			sess.Save()
		}
		return c.Redirect(c.Get("Referer", "/"))
	})

	// 材料リストを完全にクリアしてトップへ戻るルート
	app.Get("/ingredients/clear", func(c *fiber.Ctx) error {
		sess, _ := store.Get(c)
		sess.Delete("ingredients")
		sess.Save()
		return c.Redirect("/")
	})

	// 食材の重量を更新
	app.Post("/ingredients/update", func(c *fiber.Ctx) error {
		sess, _ := store.Get(c)
		ingredients := getIngredients(sess)

		id := c.FormValue("id")
		weight, _ := strconv.ParseFloat(c.FormValue("weight"), 64)
		log.Printf("[ACTION: UPDATE] ID: %s, New Weight: %.1f", id, weight)
		for i := range ingredients {
			itemIDStr := strings.TrimSuffix(fmt.Sprintf("%v", ingredients[i]["num_id"]), ".0")
			if itemIDStr == id {
				ingredients[i]["weight"] = weight
				break
			}
		}
		data, _ := json.Marshal(ingredients)
		sess.Set("ingredients", string(data))
		sess.Save()
		return c.Redirect(c.Get("Referer", "/"))
	})

	// 今の食事リストを履歴として保存
	app.Post("/history/record", func(c *fiber.Ctx) error {
		sess, _ := store.Get(c)
		ingredients := getIngredients(sess)
		if len(ingredients) == 0 {
			return c.Redirect("/")
		}

		// 保存用データの構築
		record := map[string]interface{}{
			"Ingredients": ingredients,
			"TotalItems":  len(ingredients),
		}

		// 認証済みクライアントの取得
		var client *http.Client
		if rawToken := sess.Get("oauth_token"); rawToken != nil {
			var token oauth2.Token
			if err := json.Unmarshal([]byte(rawToken.(string)), &token); err == nil {
				client = conf.Client(c.Context(), &token)
			}
		}

		if client == nil {
			return c.Status(401).SendString("履歴の保存にはログインが必要です")
		}

		if err := services.SaveMealHistory(c.Context(), client, record); err != nil {
			log.Printf("[ERROR] Failed to save meal history: %v", err)
			return c.Status(500).SendString("履歴の保存に失敗しました")
		}

		// 保存後はリストをクリアする（任意）
		sess.Delete("ingredients")
		sess.Save()

		return c.Redirect("/")
	})

	// 食材をリストから削除
	app.Post("/ingredients/remove", func(c *fiber.Ctx) error {
		sess, _ := store.Get(c)
		ingredients := getIngredients(sess)

		id := c.FormValue("id")
		log.Printf("[ACTION: REMOVE] ID: %s", id)
		var newList []FoodMap
		for _, item := range ingredients {
			// IDを文字列に正規化して比較
			itemIDStr := strings.TrimSuffix(fmt.Sprintf("%v", item["num_id"]), ".0")
			if itemIDStr != id {
				newList = append(newList, item)
			}
		}
		data, _ := json.Marshal(newList)
		sess.Set("ingredients", string(data))
		sess.Save()
		return c.Redirect(c.Get("Referer", "/"))
	})

	// 詳細画面などは foodHandler に委譲可能
	app.Get("/food/:id", foodHandler.Detail)
	app.Get("/calendar", foodHandler.CalendarIndex)

	// カレンダー用レシピ操作
	app.All("/calendar/recipes/add", foodHandler.AddRecipeToCalendarList)
	app.All("/calendar/recipes/remove", foodHandler.RemoveRecipeFromCalendarList)
	app.All("/calendar/add", foodHandler.AddToCalendar)
	app.Post("/calendar/remove/:id", foodHandler.RemoveFromCalendar)

	// 127.0.0.1:3000 で起動（APIの8080と分ける）
	port := "3000"
	fmt.Printf("Server started on http://127.0.0.1:%s\n", port)
	log.Fatal(app.Listen(":" + port))
}
