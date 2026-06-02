package handlers

import (
	"NutriTrack/models"
	"NutriTrack/services"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/session"
	"golang.org/x/oauth2"
)

type FoodHandler struct {
	Store       *session.Store
	OAuthConfig *oauth2.Config
}

// getAPIBaseURL は環境変数からAPIのベースURLを取得します
func (h *FoodHandler) getAPIBaseURL() string {
	host := os.Getenv("API_HOST")
	if host == "" {
		host = "127.0.0.1"
	}
	port := os.Getenv("API_PORT")
	if port == "" {
		port = "8080"
	}
	return fmt.Sprintf("http://%s:%s", host, port)
}

// NutrientSummary は合計栄養素を保持する構造体です
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
	VitB   float64 `json:"vitamin_b"`
	VitC   float64 `json:"vitamin_c"`
	Fiber  float64 `json:"fiber"`
	Salt   float64 `json:"salt"`
}

// Ingredient は材料リストのアイテム構造体です
type Ingredient struct {
	ID        string  `json:"num_id"`
	Name      string  `json:"name"`
	Weight    float64 `json:"weight"`
	GroupName string  `json:"group_name"`
}

// Index は一覧と検索を表示します
func (h *FoodHandler) Index(c *fiber.Ctx) error {
	query := c.Query("q")
	recipeQuery := c.Query("rq")
	sess, _ := h.Store.Get(c)
	user := sess.Get("username")

	recipes := []map[string]interface{}{}

	// ログイン中であればスプレッドシートからレシピを取得
	if rawToken := sess.Get("oauth_token"); rawToken != nil {
		var token oauth2.Token
		json.Unmarshal([]byte(rawToken.(string)), &token)
		client := h.OAuthConfig.Client(context.Background(), &token)

		data, err := services.FetchRecipes(c.Context(), client, recipeQuery)
		if err == nil {
			recipes = data
		}

	}

	// 食品検索（Nutrient APIを使用）
	var foods []map[string]interface{}
	if query != "" {
		apiUrl := fmt.Sprintf("%s/api/foods/search?q=%s", h.getAPIBaseURL(), query)
		resp, err := http.Get(apiUrl)
		if err == nil {
			defer resp.Body.Close()
			json.NewDecoder(resp.Body).Decode(&foods)
		}
	}

	return c.Render("index", fiber.Map{
		"Title":         "食品栄養素データベース",
		"User":          user,
		"Foods":         foods,
		"Query":         query,
		"RecipeQuery":   recipeQuery,
		"Ingredients":   h.getIngredientsFromSession(c),
		"MyIngredients": h.getMyIngredients(c),
		"Recipes":       recipes,
	}, "layout")
}

// Detail は詳細を表示します
func (h *FoodHandler) Detail(c *fiber.Ctx) error {
	id := c.Params("id")
	sess, _ := h.Store.Get(c)
	user := sess.Get("username")

	// Nutrient API から詳細情報を取得
	apiUrl := fmt.Sprintf("%s/api/foods/search?id=%s", h.getAPIBaseURL(), id)
	resp, err := http.Get(apiUrl)
	if err != nil || resp.StatusCode != 200 {
		return c.Status(500).SendString(err.Error())
	}
	defer resp.Body.Close()

	var results []map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&results)
	food := results[0]

	return c.Render("detail", fiber.Map{
		"Title":         "詳細情報",
		"User":          user,
		"Food":          food,
		"Ingredients":   h.getIngredientsFromSession(c),
		"MyIngredients": h.getMyIngredients(c),
	}, "layout")
}

// addNutrientsToSummary は栄養素をサマリーに加算する共通ヘルパー
func (h *FoodHandler) addNutrientsToSummary(target *NutrientSummary, data map[string]interface{}, ratio float64) {
	getVal := func(keys ...string) float64 {
		for _, k := range keys {
			if val, ok := data[k]; ok && val != nil {
				switch v := val.(type) {
				case float64:
					return v
				case int64:
					return float64(v)
				case string:
					cleaned := strings.TrimSpace(v)
					if cleaned == "-" || cleaned == "Tr" || cleaned == "(0)" {
						return 0
					}
					f, _ := strconv.ParseFloat(cleaned, 64)
					return f
				}
			}
		}
		return 0
	}
	target.Energy += getVal("enerc_kcal", "enerc") * ratio
	target.Prot += getVal("prot_", "prot", "protein") * ratio
	target.Fat += getVal("fat_", "fat") * ratio
	target.Cho += getVal("choavl", "chocdf", "cho") * ratio
	target.K += getVal("k", "potassium") * ratio
	target.Ca += getVal("ca", "calcium") * ratio
	target.Fe += getVal("fe", "iron") * ratio
	target.Zn += getVal("zn", "zinc") * ratio
	target.VitA += getVal("vita_rae", "vita") * ratio
	target.VitB += getVal("thia", "vitb1") * ratio
	target.VitC += getVal("vitc", "vitamin_c") * ratio
	target.Fiber += getVal("fib_", "fibtg", "fiber") * ratio

	salt := getVal("nacl_eq", "salt")
	if salt == 0 {
		salt = getVal("na", "sodium") * 2.54 / 1000.0
	}
	target.Salt += salt * ratio
}

// AddIngredient は材料リストにアイテムを追加します
func (h *FoodHandler) AddIngredient(c *fiber.Ctx) error {
	id := c.FormValue("id")
	name := c.FormValue("name")

	ingredients := h.getIngredientsFromSession(c)

	// 重複チェック（任意）
	for _, item := range ingredients {
		if item.ID == id {
			return c.Redirect(c.Get("Referer", "/"))
		}
	}

	ingredients = append(ingredients, Ingredient{ID: id, Name: name})
	sess, _ := h.Store.Get(c)
	data, _ := json.Marshal(ingredients)
	sess.Set("ingredients", string(data))
	sess.Save()

	return c.Redirect(c.Get("Referer", "/"))
}

// SearchJSON は食品を検索し JSON 形式で返します
func (h *FoodHandler) SearchJSON(c *fiber.Ctx) error {
	query := c.Query("q")
	var foods []map[string]interface{}
	apiUrl := fmt.Sprintf("%s/api/foods/search?q=%s", h.getAPIBaseURL(), query)
	resp, err := http.Get(apiUrl)
	if err == nil {
		defer resp.Body.Close()
		json.NewDecoder(resp.Body).Decode(&foods)
	}
	return c.JSON(foods)
}

// SearchRecipesJSON はレシピを検索し JSON 形式で返します
func (h *FoodHandler) SearchRecipesJSON(c *fiber.Ctx) error {
	query := c.Query("q")
	// スプレッドシート連携までモックを返す
	log.Printf("Recipe search requested for: %s", query)
	return c.JSON([]interface{}{})
}

// RemoveIngredient は材料リストからアイテムを削除します
func (h *FoodHandler) RemoveIngredient(c *fiber.Ctx) error {
	id := c.Params("id")

	ingredients := h.getIngredientsFromSession(c)

	newIngredients := []Ingredient{}
	for _, item := range ingredients {
		if item.ID != id {
			newIngredients = append(newIngredients, item)
		}
	}

	data, _ := json.Marshal(newIngredients)
	sess, _ := h.Store.Get(c)
	if len(newIngredients) == 0 {
		sess.Delete("ingredients")
	} else {
		sess.Set("ingredients", string(data))
	}
	_ = sess.Save()

	return c.Redirect(c.Get("Referer", "/"))
}

// NewRecipe はレシピ作成画面を表示します
func (h *FoodHandler) NewRecipe(c *fiber.Ctx) error {
	query := c.Query("q")
	sess, _ := h.Store.Get(c)
	user := sess.Get("username")
	rawToken := sess.Get("oauth_token")

	ingredients := h.getIngredientsFromSession(c)

	// 検索クエリがある場合は食品を検索
	var foods []map[string]interface{}
	if query != "" {
		apiUrl := fmt.Sprintf("%s/api/foods/search?q=%s", h.getAPIBaseURL(), query)
		resp, _ := http.Get(apiUrl)
		if resp != nil {
			defer resp.Body.Close()
			json.NewDecoder(resp.Body).Decode(&foods)
		}
	}

	// 栄養素の加算用ヘルパー (EditRecipeと同様)
	summary := NutrientSummary{}
	addVal := func(target *float64, data map[string]interface{}, wRatio float64, keys ...string) {
		for _, k := range keys {
			if val, ok := data[k]; ok && val != nil {
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
	}

	// 現在のセッション材料からチャート用に計算
	for _, ing := range ingredients {
		apiUrl := fmt.Sprintf("%s/api/foods/search?id=%s", h.getAPIBaseURL(), ing.ID)
		resp, err := http.Get(apiUrl)
		if err == nil {
			var details []map[string]interface{}
			if decodeErr := json.NewDecoder(resp.Body).Decode(&details); decodeErr == nil && len(details) > 0 {
				data := details[0]
				wRatio := ing.Weight / 100.0
				addVal(&summary.Energy, data, wRatio, "enerc_kcal", "enerc")
				addVal(&summary.Prot, data, wRatio, "prot_", "prot", "protein")
				addVal(&summary.Fat, data, wRatio, "fat_", "fat")
				addVal(&summary.Cho, data, wRatio, "choavl", "chocdf", "cho")
				addVal(&summary.K, data, wRatio, "k", "potassium")
				addVal(&summary.Ca, data, wRatio, "ca", "calcium")
				addVal(&summary.Fe, data, wRatio, "fe", "iron")
				addVal(&summary.Zn, data, wRatio, "zn", "zinc")
				addVal(&summary.VitA, data, wRatio, "vita_rae", "vita")
				addVal(&summary.VitB, data, wRatio, "thia", "vitb1")
				addVal(&summary.VitC, data, wRatio, "vitc", "vitamin_c")
				addVal(&summary.Fiber, data, wRatio, "fib_", "fibtg", "fiber")
			}
			if resp != nil {
				resp.Body.Close()
			}
		}
	}

	var groups []string
	if rawToken != nil {
		var token oauth2.Token
		if err := json.Unmarshal([]byte(rawToken.(string)), &token); err == nil {
			client := h.OAuthConfig.Client(c.Context(), &token)
			groups, _ = services.ListRecipeGroups(c.Context(), client)
		}
	}

	return c.Render("recipe_edit", fiber.Map{
		"Title":         "レシピ作成",
		"User":          user,
		"Recipe":        map[string]interface{}{}, // 空のレシピ
		"Groups":        groups,
		"Foods":         foods,
		"Query":         query,
		"Summary":       summary,
		"Ingredients":   ingredients,
		"MyIngredients": h.getMyIngredients(c),
		"IsRecipePage":  true,
	}, "layout")
}

// CreateRecipe はレシピをデータベースに保存します
func (h *FoodHandler) CreateRecipe(c *fiber.Ctx) error {
	sess, _ := h.Store.Get(c)
	userID, _ := sess.Get("user_id").(string)
	log.Printf("[DEBUG] CreateRecipe: UserID=%s", userID)
	rawToken := sess.Get("oauth_token")

	if rawToken == nil {
		return c.Status(401).SendString("認証が必要です")
	}

	title := c.FormValue("title")
	group := c.FormValue("group")
	description := c.FormValue("description")

	// 調理手順の取得
	stepsRaw := c.Request().PostArgs().PeekMulti("steps")
	steps := make([]string, len(stepsRaw))
	for i, s := range stepsRaw {
		steps[i] = string(s)
	}

	// 材料の詳細（IDと最新の重量）をフォームから取得
	ingIDs := c.Request().PostArgs().PeekMulti("ingredient_ids")
	var ingredientList []map[string]interface{}

	// セッションから名前等を取得するためのマップ
	sessionIngs := h.getIngredientsFromSession(c)
	ingMap := make(map[string]Ingredient)
	for _, v := range sessionIngs {
		ingMap[v.ID] = v
	}

	for _, idByte := range ingIDs {
		id := string(idByte)
		weight, _ := strconv.ParseFloat(c.FormValue("qty_"+id), 64)
		group := c.FormValue("grp_" + id)
		name := id
		if val, ok := ingMap[id]; ok {
			name = val.Name
		}

		ingredientList = append(ingredientList, map[string]interface{}{
			"ID":        id,
			"Name":      name,
			"Quantity":  weight,
			"GroupName": group,
		})
	}

	// OAuth2 クライアントの作成
	var token oauth2.Token
	json.Unmarshal([]byte(rawToken.(string)), &token)
	client := h.OAuthConfig.Client(context.Background(), &token)

	// タイムスタンプをコンテキストに含めて渡す
	ctx := context.WithValue(context.Background(), "timestamp", time.Now().Format("2006-01-02 15:04:05"))

	// シートごとに1レシピを保存 (シート名 = レシピ名称)
	err := services.CreateRecipeJSON(ctx, client, title, group, description, ingredientList, steps)
	if err != nil {
		log.Printf("CreateRecipeJSON Error: %v", err)
		return c.Status(500).SendString("JSONへの保存に失敗しました: " + err.Error())
	}

	// セッションの材料リストをクリア
	sess.Delete("ingredients")
	sess.Save()
	return c.Redirect("/")
}

// EditRecipe はレシピの編集画面を表示します
func (h *FoodHandler) EditRecipe(c *fiber.Ctx) error {
	id := c.Params("id")
	log.Printf("DEBUG: EditRecipe called with ID: %s", id)
	query := c.Query("q")
	sess, _ := h.Store.Get(c)
	user := sess.Get("username")
	rawToken := sess.Get("oauth_token")

	var recipe map[string]interface{}
	var token oauth2.Token
	hasToken := false

	if rawToken != nil && json.Unmarshal([]byte(rawToken.(string)), &token) == nil {
		hasToken = true
		client := h.OAuthConfig.Client(c.Context(), &token)
		// JSONファイルから全レシピを取得してIDで検索
		recipes, err := services.FetchRecipes(c.Context(), client, "")
		if err == nil {
			for _, r := range recipes {
				if fmt.Sprintf("%v", r["ID"]) == id {
					recipe = r
					break
				}
			}
		}
	}

	if recipe == nil {
		return c.Redirect("/")
	}

	// 工程（Steps）のデータ形式をテンプレート用に正規化（古い形式の互換性維持）
	if rawSteps, ok := recipe["Steps"].([]interface{}); ok {
		normalized := make([]map[string]interface{}, 0, len(rawSteps))
		for i, s := range rawSteps {
			switch v := s.(type) {
			case string:
				normalized = append(normalized, map[string]interface{}{
					"StepNumber":  i + 1,
					"Instruction": v,
				})
			case map[string]interface{}:
				normalized = append(normalized, v)
			}
		}
		recipe["Steps"] = normalized
	}

	// 検索クエリがある場合は食品を検索
	var foods []map[string]interface{}
	if query != "" {
		apiUrl := fmt.Sprintf("%s/api/foods/search?q=%s", h.getAPIBaseURL(), query)
		resp, _ := http.Get(apiUrl)
		defer resp.Body.Close()
		json.NewDecoder(resp.Body).Decode(&foods)
	}

	// 栄養素の加算用ヘルパー
	summary := NutrientSummary{}
	addVal := func(target *float64, data map[string]interface{}, wRatio float64, keys ...string) {
		for _, k := range keys {
			if val, ok := data[k]; ok && val != nil {
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
	}

	// セッションが空の場合、検索の有無に関わらずDBから材料をロードする
	// これにより、検索(q=...)実行時でも材料リストが維持される
	// ユーザー要望: 編集画面を開いたときは、そのレシピの材料だけを表示し、セッションを上書きする
	var ingredients []Ingredient
	if rawIngs, ok := recipe["Ingredients"].([]interface{}); ok {
		for _, rawIng := range rawIngs {
			if ing, ok := rawIng.(map[string]interface{}); ok {
				// APIから栄養素を取得
				ingID := fmt.Sprintf("%v", ing["ID"])
				apiUrl := fmt.Sprintf("%s/api/foods/search?id=%s", h.getAPIBaseURL(), ingID)
				resp, err := http.Get(apiUrl)
				if err == nil {
					var details []map[string]interface{}
					if decodeErr := json.NewDecoder(resp.Body).Decode(&details); decodeErr == nil && len(details) > 0 {
						data := details[0]

						qty, _ := ing["Quantity"].(float64)
						if qty == 0 {
							qty, _ = ing["Weight"].(float64)
						}
						wRatio := qty / 100.0

						addVal(&summary.Energy, data, wRatio, "enerc_kcal", "enerc")
						addVal(&summary.Prot, data, wRatio, "prot_", "prot", "protein")
						addVal(&summary.Fat, data, wRatio, "fat_", "fat")
						addVal(&summary.Cho, data, wRatio, "choavl", "chocdf", "cho")
						addVal(&summary.K, data, wRatio, "k", "potassium")
						addVal(&summary.Ca, data, wRatio, "ca", "calcium")
						addVal(&summary.Fe, data, wRatio, "fe", "iron")
						addVal(&summary.Zn, data, wRatio, "zn", "zinc")
						addVal(&summary.VitA, data, wRatio, "vita_rae", "vita")
						addVal(&summary.VitB, data, wRatio, "thia", "vitb1")
						addVal(&summary.VitC, data, wRatio, "vitc", "vitamin_c")
						addVal(&summary.Fiber, data, wRatio, "fib_", "fibtg", "fiber")

						var salt float64
						addVal(&salt, data, wRatio, "nacl_eq", "salt")
						if salt == 0 {
							var na float64
							addVal(&na, data, wRatio, "na", "sodium")
							salt = na * 2.54 / 1000.0
						}
						summary.Salt += salt
					}
					resp.Body.Close()
				}

				qty, _ := ing["Quantity"].(float64)
				if qty == 0 {
					qty, _ = ing["Weight"].(float64)
				}
				grp, _ := ing["GroupName"].(string)
				if grp == "" {
					grp, _ = ing["Group"].(string)
				}

				ingredients = append(ingredients, Ingredient{
					ID:        fmt.Sprintf("%v", ing["ID"]),
					Name:      fmt.Sprintf("%v", ing["Name"]),
					Weight:    qty,
					GroupName: grp,
				})
			}
		}
	}
	data, _ := json.Marshal(ingredients)
	sess.Set("ingredients", string(data))
	_ = sess.Save()

	var groups []string
	if hasToken {
		client := h.OAuthConfig.Client(c.Context(), &token)
		groups, _ = services.ListRecipeGroups(c.Context(), client)
	}

	return c.Render("recipe_edit", fiber.Map{
		"Title":         "レシピ編集",
		"User":          user,
		"Recipe":        recipe,
		"Groups":        groups,
		"Foods":         foods,
		"Query":         query,
		"Summary":       summary,
		"Ingredients":   h.getIngredientsFromSession(c),
		"MyIngredients": h.getMyIngredients(c),
		"IsRecipePage":  true,
	}, "layout")
}

func (h *FoodHandler) DeleteRecipe(c *fiber.Ctx) error {
	id := c.Params("id")
	group := c.Params("group")
	sess, _ := h.Store.Get(c)
	rawToken := sess.Get("oauth_token")

	if rawToken == nil {
		return c.Status(401).SendString("認証が必要です")
	}

	var token oauth2.Token
	json.Unmarshal([]byte(rawToken.(string)), &token)
	client := h.OAuthConfig.Client(context.Background(), &token)

	err := services.DeleteRecipe(c.Context(), client, id, group)
	if err != nil {
		log.Printf("DeleteRecipe Error: %v", err)
		return c.Status(500).SendString("レシピの削除に失敗しました: " + err.Error())
	}

	return c.Redirect("/")
}

// UpdateRecipe はレシピを更新します
func (h *FoodHandler) UpdateRecipe(c *fiber.Ctx) error {
	id := c.Params("id")
	sess, _ := h.Store.Get(c)
	rawToken := sess.Get("oauth_token")

	if rawToken == nil {
		return c.Status(401).SendString("認証が必要です")
	}

	title := c.FormValue("title")
	group := c.FormValue("group")
	description := c.FormValue("description")

	// 調理手順の取得
	stepsRaw := c.Request().PostArgs().PeekMulti("steps")
	steps := make([]string, len(stepsRaw))
	for i, s := range stepsRaw {
		steps[i] = string(s)
	}

	// 材料の詳細を取得
	ingIDs := c.Request().PostArgs().PeekMulti("ingredient_ids")
	var ingredientList []map[string]interface{}

	sessionIngs := h.getIngredientsFromSession(c)
	ingMap := make(map[string]Ingredient)
	for _, v := range sessionIngs {
		ingMap[v.ID] = v
	}

	for _, idByte := range ingIDs {
		ingID := string(idByte)
		weight, _ := strconv.ParseFloat(c.FormValue("qty_"+ingID), 64)
		group := c.FormValue("grp_" + ingID)
		name := ingID
		if val, ok := ingMap[ingID]; ok {
			name = val.Name
		}

		ingredientList = append(ingredientList, map[string]interface{}{
			"ID":        ingID,
			"Name":      name,
			"Quantity":  weight,
			"GroupName": group,
		})
	}

	var token oauth2.Token
	json.Unmarshal([]byte(rawToken.(string)), &token)
	client := h.OAuthConfig.Client(context.Background(), &token)

	err := services.UpdateRecipe(c.Context(), client, id, group, title, description, ingredientList, steps)
	if err != nil {
		log.Printf("UpdateRecipe Error: %v", err)
		return c.Status(500).SendString("レシピの更新に失敗しました: " + err.Error())
	}

	// セッションの材料リストをクリア
	sess.Delete("ingredients")
	sess.Save()

	return c.Redirect("/recipe/" + id)
}

// getIngredientsFromSession はセッションから材料リストを取得するヘルパーメソッドです
func (h *FoodHandler) getIngredientsFromSession(c *fiber.Ctx) []Ingredient {
	sess, _ := h.Store.Get(c)
	var ingredients []Ingredient
	if raw := sess.Get("ingredients"); raw != nil {
		json.Unmarshal([]byte(raw.(string)), &ingredients)
	}

	// 材料リストのソート: グループなしを最優先し、次にグループ名、最後に名称でソート
	sort.Slice(ingredients, func(i, j int) bool {
		emptyI := ingredients[i].GroupName == ""
		emptyJ := ingredients[j].GroupName == ""
		if emptyI != emptyJ {
			return emptyI // iが空ならtrueを返し、jより前にくる
		}
		// グループ名で比較
		if ingredients[i].GroupName != ingredients[j].GroupName {
			return ingredients[i].GroupName < ingredients[j].GroupName
		}
		// 名称で比較
		return ingredients[i].Name < ingredients[j].Name
	})

	return ingredients
}

// getMyIngredients はユーザーの履歴またはデフォルトの材料を取得します
func (h *FoodHandler) getMyIngredients(c *fiber.Ctx) []models.Food {
	var myIngredients []models.Food
	if len(myIngredients) == 0 {
		data, err := os.ReadFile("./data/default_ingredients.json")
		if err == nil {
			json.Unmarshal(data, &myIngredients)
		} else {
			log.Println("Warning: Could not load default_ingredients.json:", err)
		}
	}
	return myIngredients
}

// getSelectedRecipes はセッションから選択中のレシピを取得するヘルパーです
func (h *FoodHandler) getSelectedRecipes(c *fiber.Ctx) []map[string]string {
	sess, _ := h.Store.Get(c)
	var recipes []map[string]string
	if raw := sess.Get("calendar_recipes"); raw != nil {
		if err := json.Unmarshal([]byte(raw.(string)), &recipes); err != nil {
			log.Printf("[ERROR] Failed to unmarshal calendar_recipes: %v", err)
		}
	}
	return recipes
}

// CalendarIndex はカレンダー画面を表示します
func (h *FoodHandler) CalendarIndex(c *fiber.Ctx) error {
	sess, _ := h.Store.Get(c)
	user := sess.Get("username")
	userID, ok := sess.Get("user_id").(string)
	if !ok {
		return c.Redirect("/login")
	}
	log.Printf("[DEBUG] CalendarIndex: UserID=%s", userID)

	dateStr := c.Query("date")
	recipeQuery := c.Query("rq")
	if dateStr == "" {
		dateStr = time.Now().Format("2006-01-02")
	}

	// Google Drive から履歴を取得
	var entries []map[string]interface{}
	recipes := []map[string]interface{}{}
	totalCalories := 0.0
	selectedSummary := NutrientSummary{}
	steps := 0
	burned := 0
	healthSynced := false

	if rawToken := sess.Get("oauth_token"); rawToken != nil {
		var token oauth2.Token
		json.Unmarshal([]byte(rawToken.(string)), &token)
		client := h.OAuthConfig.Client(context.Background(), &token)

		// 1. 食事履歴の取得
		history, err := services.FetchMealHistory(c.Context(), client)
		if err == nil {
			for _, record := range history {
				ts, _ := record["Timestamp"].(string)
				if strings.HasPrefix(ts, dateStr) {
					entries = append(entries, record)
					if summ, ok := record["Summary"].(map[string]interface{}); ok {
						cal, _ := summ["energy"].(float64)
						totalCalories += cal
					}
				}
			}
		}

		// 2. レシピ検索処理
		data, err := services.FetchRecipes(c.Context(), client, recipeQuery)
		if err == nil {
			// 各検索結果レシピの栄養素を計算 (チャート1用)
			for _, r := range data {
				summ := NutrientSummary{}
				if ings, ok := r["Ingredients"].([]interface{}); ok {
					for _, rawIng := range ings {
						if ing, ok := rawIng.(map[string]interface{}); ok {
							ingID := fmt.Sprintf("%v", ing["ID"])
							u := fmt.Sprintf("%s/api/foods/search?id=%s", h.getAPIBaseURL(), ingID)
							if resp, err := http.Get(u); err == nil {
								var details []map[string]interface{}
								if de := json.NewDecoder(resp.Body).Decode(&details); de == nil && len(details) > 0 {
									d := details[0]
									qty, ok := ing["Quantity"].(float64)
									if !ok {
										qty, _ = ing["Weight"].(float64)
									}
									h.addNutrientsToSummary(&summ, d, qty/100.0)
								}
								resp.Body.Close()
							}
						}
					}
				}
				r["Summary"] = summ
				recipes = append(recipes, r)
			}
		}

		// 3. 選択中レシピの栄養素計算
		selectedRecipesRaw := h.getSelectedRecipes(c)
		enrichedSelectedRecipes := []map[string]interface{}{}
		if len(selectedRecipesRaw) > 0 {
			// 全レシピを取得してマップ化
			recipeMap := make(map[string]map[string]interface{})
			allRecipes, _ := services.FetchRecipes(c.Context(), client, "")
			for _, r := range allRecipes {
				recipeMap[fmt.Sprintf("%v", r["ID"])] = r
			}

			for _, sr := range selectedRecipesRaw {
				itemSummary := NutrientSummary{}
				recipeID := sr["id"]
				recipeName := sr["name"]

				if r, ok := recipeMap[recipeID]; ok {
					if ings, ok := r["Ingredients"].([]interface{}); ok {
						for _, rawIng := range ings {
							if ing, ok := rawIng.(map[string]interface{}); ok {
								// 最新栄養素を取得
								u := fmt.Sprintf("%s/api/foods/search?id=%s", h.getAPIBaseURL(), fmt.Sprintf("%v", ing["ID"]))
								if resp, err := http.Get(u); err == nil {
									var details []map[string]interface{}
									if de := json.NewDecoder(resp.Body).Decode(&details); de == nil && len(details) > 0 {
										data := details[0]
										qty, ok := ing["Quantity"].(float64)
										if !ok {
											qty, _ = ing["Weight"].(float64)
										}
										h.addNutrientsToSummary(&itemSummary, data, qty/100.0)
										h.addNutrientsToSummary(&selectedSummary, data, qty/100.0)
									}
									resp.Body.Close()
								}
							}
						}
					}
				}
				enrichedSelectedRecipes = append(enrichedSelectedRecipes, map[string]interface{}{
					"id":      recipeID,
					"name":    recipeName,
					"Summary": itemSummary,
				})
			}
		}

		return c.Render("calendar", fiber.Map{
			"Title":                "食事カレンダー",
			"User":                 user,
			"Date":                 dateStr,
			"Entries":              entries,
			"Recipes":              recipes,
			"SelectedRecipes":      enrichedSelectedRecipes,
			"SelectedSummary":      selectedSummary,
			"RecipeQuery":          recipeQuery,
			"TotalIntake":          int(totalCalories),
			"BurnedCalories":       burned,
			"Steps":                steps,
			"HealthSynced":         healthSynced,
			"HideIngredientDrawer": true,
			"Ingredients":          h.getIngredientsFromSession(c),
		}, "layout")
	}

	return c.Redirect("/login")
}

// AddRecipeToCalendarList はカレンダー登録待ちリストにレシピを追加します
func (h *FoodHandler) AddRecipeToCalendarList(c *fiber.Ctx) error {
	// FiberのFormValueはQueryとBodyの両方をチェックするため、GET/POSTどちらでも対応可能
	id := c.FormValue("id")
	name := c.FormValue("name")

	sess, _ := h.Store.Get(c)
	var recipes []map[string]string
	if raw := sess.Get("calendar_recipes"); raw != nil {
		json.Unmarshal([]byte(raw.(string)), &recipes)
	}

	// 重複チェック
	for _, r := range recipes {
		if r["id"] == id {
			return c.Redirect(c.Get("Referer", "/calendar"))
		}
	}

	recipes = append(recipes, map[string]string{"id": id, "name": name})
	data, _ := json.Marshal(recipes)
	sess.Set("calendar_recipes", string(data))
	sess.Save()

	return c.Redirect(c.Get("Referer", "/calendar"))
}

// RemoveRecipeFromCalendarList はカレンダー登録待ちリストからレシピを削除します
func (h *FoodHandler) RemoveRecipeFromCalendarList(c *fiber.Ctx) error {
	id := c.FormValue("id")
	sess, _ := h.Store.Get(c)

	var recipes []map[string]string
	if raw := sess.Get("calendar_recipes"); raw != nil {
		json.Unmarshal([]byte(raw.(string)), &recipes)
	}

	newRecipes := []map[string]string{}
	for _, r := range recipes {
		if r["id"] != id {
			newRecipes = append(newRecipes, r)
		}
	}

	if len(newRecipes) == 0 {
		sess.Delete("calendar_recipes")
	} else {
		data, _ := json.Marshal(newRecipes)
		sess.Set("calendar_recipes", string(data))
	}
	sess.Save()

	return c.Redirect(c.Get("Referer", "/calendar"))
}

// AddToCalendar はレシピをカレンダーに登録します
func (h *FoodHandler) AddToCalendar(c *fiber.Ctx) error {
	sess, _ := h.Store.Get(c)
	rawToken := sess.Get("oauth_token")
	if rawToken == nil {
		return c.Status(401).SendString("認証が必要です")
	}

	mealType := c.FormValue("meal_type")   // breakfast, lunch, dinner
	date := c.FormValue("date")            // YYYY-MM-DD
	entryTime := c.FormValue("entry_time") // HH:mm

	if date == "" {
		date = time.Now().Format("2006-01-02")
	}

	// 間食の場合で時刻が空なら、日本時間の現在時刻をセットする
	if entryTime == "" && mealType == "snack" {
		entryTime = time.Now().In(time.FixedZone("Asia/Tokyo", 9*60*60)).Format("15:04")
	} else if entryTime == "" {
		entryTime = "12:00"
	}

	// 1. セッションから登録対象のレシピを取得
	var selectedRecipes []map[string]string
	if raw := sess.Get("calendar_recipes"); raw != nil {
		json.Unmarshal([]byte(raw.(string)), &selectedRecipes)
	}

	if len(selectedRecipes) == 0 {
		return c.Redirect("/calendar?date=" + date)
	}

	var token oauth2.Token
	json.Unmarshal([]byte(rawToken.(string)), &token)
	client := h.OAuthConfig.Client(context.Background(), &token)

	// 2. 登録する全レシピの合計栄養素を計算
	summary := NutrientSummary{}
	allRecipes, err := services.FetchRecipes(c.Context(), client, "")
	if err != nil {
		return c.Status(500).SendString("レシピの取得に失敗しました")
	}

	recipeMap := make(map[string]map[string]interface{})
	for _, r := range allRecipes {
		recipeMap[fmt.Sprintf("%v", r["ID"])] = r
	}

	// 栄養素加算用ヘルパー
	addVal := func(target *float64, data map[string]interface{}, wRatio float64, keys ...string) {
		for _, k := range keys {
			if val, ok := data[k]; ok && val != nil {
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
	}

	// 選択されたレシピごとに材料をスキャン
	for _, sr := range selectedRecipes {
		if r, ok := recipeMap[sr["id"]]; ok {
			if ings, ok := r["Ingredients"].([]interface{}); ok {
				for _, rawIng := range ings {
					if ing, ok := rawIng.(map[string]interface{}); ok {
						ingID := fmt.Sprintf("%v", ing["ID"])
						apiUrl := fmt.Sprintf("%s/api/foods/search?id=%s", h.getAPIBaseURL(), ingID)
						resp, err := http.Get(apiUrl)
						if err == nil && resp.StatusCode == 200 {
							var details []map[string]interface{}
							if err := json.NewDecoder(resp.Body).Decode(&details); err == nil && len(details) > 0 {
								nutrientData := details[0]

								qty, ok := ing["Quantity"].(float64)
								if !ok {
									qty, _ = ing["Weight"].(float64)
								}
								wRatio := qty / 100.0

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
									var na float64
									addVal(&na, nutrientData, wRatio, "na", "sodium")
									salt = na * 2.54 / 1000.0
								}
								summary.Salt += salt
							}
							resp.Body.Close()
						}
					}
				}
			}
		}
	}

	// 3. カレンダー用データ構造の構築
	record := map[string]interface{}{
		"MealType":  mealType,
		"Recipes":   selectedRecipes,
		"Summary":   summary,
		"Timestamp": date + "T" + entryTime + ":00Z", // RFC3339形式
	}

	services.SaveMealHistory(c.Context(), client, record)

	// 完了後、セッションをクリア
	sess.Delete("calendar_recipes")
	sess.Save()

	return c.Redirect("/calendar?date=" + date)
}

// RemoveFromCalendar はカレンダーから特定の食事記録を削除します
func (h *FoodHandler) RemoveFromCalendar(c *fiber.Ctx) error {
	id := c.Params("id")
	sess, _ := h.Store.Get(c)
	userID, ok := sess.Get("user_id").(string)
	if !ok {
		return c.Redirect("/login")
	}

	// 日付を取得しておく（リダイレクトと同期フラグ更新用）
	var date string
	// TODO: スプレッドシートから対象のエントリを削除
	log.Printf("RemoveFromCalendar (WIP): User %s removed entry %s", userID, id)

	if date == "" {
		date = time.Now().Format("2006-01-02")
	}

	return c.Redirect("/calendar?date=" + date)
}

// syncNutritionToFit はレシピの栄養素を Google Fit に書き込みます
func (h *FoodHandler) syncNutritionToFit(
	c *fiber.Ctx,
	title string,
	calories, protein, fat, carbs float64,
	dateStr string,
	timeStr string,
	mealType int,
) {
	sess, _ := h.Store.Get(c)
	userID, _ := sess.Get("user_id").(string)
	rawToken := sess.Get("oauth_token")
	var token oauth2.Token
	_ = json.Unmarshal([]byte(rawToken.(string)), &token)

	fitDataSourceID, err := h.getOrCreateFitDataSource(userID, token)
	if err != nil {
		log.Printf("Fit Data Source Error: %v", err)
		return
	}

	client := h.OAuthConfig.Client(context.Background(), &token)

	// 日本時間 (JST) として日付を解析
	jst := time.FixedZone("Asia/Tokyo", 9*60*60)
	// dateStrに時刻が含まれている場合に備え、先頭10文字(YYYY-MM-DD)のみ使用
	cleanDate := dateStr
	if len(dateStr) > 10 {
		cleanDate = dateStr[:10]
	}

	// 保存された時刻を使用して開始時間を計算
	startTime, err := time.ParseInLocation("2006-01-02 15:04", cleanDate+" "+timeStr, jst)
	if err != nil {
		log.Printf("WARNING: Invalid time format '%s' for '%s'. Using fallback.", timeStr, title)
		// 解析に失敗した場合は食事区分に基づいたデフォルト時刻を採用
		hour := 15
		switch mealType {
		case 1:
			hour = 8
		case 2:
			hour = 12
		case 3:
			hour = 18
		}
		t, _ := time.ParseInLocation("2006-01-02", cleanDate, jst)
		startTime = time.Date(t.Year(), t.Month(), t.Day(), hour, 0, 0, 0, jst)
	}
	startTimeNanos := startTime.UnixNano()
	endTimeNanos := startTime.Add(1 * time.Hour).UnixNano()

	nutritionMap := map[string]float64{
		"calories":           calories,
		"protein":            protein,
		"total_fat":          fat,
		"total_carbohydrate": carbs,
	}

	requestBody := map[string]interface{}{
		"dataSourceId":   fitDataSourceID,
		"minStartTimeNs": strconv.FormatInt(startTimeNanos, 10),
		"maxEndTimeNs":   strconv.FormatInt(endTimeNanos, 10),
		"point": []map[string]interface{}{
			{
				"startTimeNanos": strconv.FormatInt(startTimeNanos, 10),
				"endTimeNanos":   strconv.FormatInt(endTimeNanos, 10),
				"dataTypeName":   "com.google.nutrition",
				"value": []map[string]interface{}{ // This is an array of Value objects
					{"mapVal": h.formatNutritionMap(nutritionMap)},
					{
						"intVal": mealType,
						"key":    "meal_type", // Add key for meal_type
					},
					{
						"stringVal": title,
						"key":       "food_item", // Add key for food_item
					},
				},
			},
		},
	}

	jsonReq, _ := json.Marshal(requestBody)
	// dataset:patch を使用してデータをアップロード
	datasetID := strconv.FormatInt(startTimeNanos, 10) + "-" + strconv.FormatInt(endTimeNanos, 10) // Dataset ID must be in nanoseconds
	url := "https://www.googleapis.com/fitness/v1/users/me/dataSources/" + fitDataSourceID + "/datasets/" + datasetID

	req, err := http.NewRequest("PATCH", url, bytes.NewBuffer(jsonReq))
	if err != nil {
		log.Printf("Google Fit Nutrition Sync Error: Failed to create request: %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	log.Printf("DEBUG: Patching nutrition to Fit for '%s' (%.2f kcal)...", title, calories)
	resp, err := client.Do(req)
	if err != nil {
		log.Println("Google Fit Nutrition Sync Error:", err)
	} else {
		if resp.StatusCode != 200 {
			body, _ := io.ReadAll(resp.Body)
			log.Printf("Google Fit Sync Failed [%d]: %s", resp.StatusCode, string(body))
		} else {
			log.Printf("SUCCESS: Nutritional data for '%s' synced. Status: %s", title, resp.Status)
		}
		defer resp.Body.Close()
	}
}

// getOrCreateFitDataSource はユーザーの Google Fit データソースIDを取得または作成します
func (h *FoodHandler) getOrCreateFitDataSource(userID string, token oauth2.Token) (string, error) {
	// 注意: 本来はスプレッドシート等から取得するロジックに置き換える必要があります
	// 現在は常に新規作成を試みるか、あるいはFit側での重複チェックに任せる形になります

	// なければ新規作成
	client := h.OAuthConfig.Client(context.Background(), &token)

	// データソース作成リクエストボディ
	createSourceBody := map[string]interface{}{
		"dataStreamName": "NutriTrack Nutrition Data",
		"type":           "raw",
		"dataType": map[string]string{
			"name": "com.google.nutrition",
		},
		"application": map[string]string{
			"detailsUrl": os.Getenv("APP_BASE_URL"), // 環境変数から取得
			"name":       "NutriTrack",
			"version":    "1.0",
		},
		// "device": map[string]string{ // Google Fit API の device.type に "platform" は無効なため削除
		// 	"manufacturer": "RecipeApp",
		// 	"model":        "Web",
		// 	"type":         "platform",
		// 	"uid":          "web-app-instance-" + strconv.Itoa(userID), // ユーザーごとにユニークなID
		// },
	}

	jsonReq, _ := json.Marshal(createSourceBody)
	resp, err := client.Post("https://www.googleapis.com/fitness/v1/users/me/dataSources", "application/json", bytes.NewBuffer(jsonReq))
	if err != nil {
		return "", fmt.Errorf("failed to create Fit data source: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("failed to create Fit data source, status: %s, body: %s", resp.Status, string(body))
	}

	var result struct {
		DataSourceID string `json:"dataStreamId"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to parse data source creation response: %w", err)
	}

	// スプレッドシート等にデータソースIDを永続化するロジックを将来的に追加
	log.Printf("Fit data source created: %s for user %s", result.DataSourceID, userID)

	return result.DataSourceID, nil
}

// Fit の形式（key: {fpVal: val}）に変換するヘルパー
func (h *FoodHandler) formatNutritionMap(m map[string]float64) []map[string]interface{} {
	var res []map[string]interface{}
	for k, v := range m {
		if v > 0 {
			res = append(res, map[string]interface{}{
				"key": k,
				"value": map[string]interface{}{
					"fpVal": v,
				},
			})
		}
	}
	return res
}

// DisconnectHealthData は健康データの連携を解除します
func (h *FoodHandler) DisconnectHealthData(c *fiber.Ctx) error {
	sess, _ := h.Store.Get(c)

	// セッションからトークン情報を削除
	sess.Delete("oauth_token")

	if err := sess.Save(); err != nil {
		return c.Status(500).SendString("連携解除に失敗しました")
	}

	return c.Redirect("/calendar")
}

// SyncHealthData は健康データを同期するスタブ（概念）
func (h *FoodHandler) SyncHealthData(c *fiber.Ctx) error {
	sess, _ := h.Store.Get(c)
	rawToken := sess.Get("oauth_token")
	if rawToken == nil {
		return c.Status(401).SendString("OAuthトークンが見つかりません。再ログインしてください。")
	}
	userID, _ := sess.Get("user_id").(string)

	// 1. 準備：同期対象の日付とタイムゾーンの設定
	dateStr := c.Query("date")
	if dateStr == "" {
		dateStr = time.Now().Format("2006-01-02")
	}
	log.Printf("DEBUG: SyncHealthData started for %s", dateStr)

	jst := time.FixedZone("Asia/Tokyo", 9*60*60)
	cleanDate := dateStr
	if len(dateStr) > 10 {
		cleanDate = dateStr[:10]
	}
	t, _ := time.ParseInLocation("2006-01-02", cleanDate, jst)
	startTime := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, jst)
	endTime := startTime.Add(24 * time.Hour).Add(-time.Nanosecond)

	var token oauth2.Token
	if err := json.Unmarshal([]byte(rawToken.(string)), &token); err != nil {
		return c.Status(500).SendString("トークンの解析に失敗しました")
	}

	mySourceID, _ := h.getOrCreateFitDataSource(userID, token)

	// 1. SQLite 側の現在の食事データを「判定用」に準備
	type mealExpectation struct {
		mealTypeStr string
		calories    float64
		startTimeNs int64
		foundInFit  bool
	}
	expectedMeals := []mealExpectation{}
	for _, mt := range []string{"breakfast", "lunch", "dinner", "snack"} {
		// TODO: スプレッドシートから栄養素情報を取得するように変更
		log.Printf("SyncHealthData (WIP): Checking nutrition for %s", mt)
		// nut, _ := models.GetMealTypeNutrition(h.DB, userID, cleanDate, mt)
		// if nut != nil && nut.TotalCalories > 0 {
		// 	sTime, err := time.ParseInLocation("2006-01-02 15:04", cleanDate+" "+nut.EntryTime, jst)
		// 	if err != nil {
		// 		hour := 15
		// 		switch mt {
		// 		case "breakfast": hour = 8
		// 		case "lunch": hour = 12
		// 		case "dinner": hour = 18
		// 		}
		// 		sTime = time.Date(t.Year(), t.Month(), t.Day(), hour, 0, 0, 0, jst)
		// 	}
		// 	expectedMeals = append(expectedMeals, mealExpectation{ ... })
		// }
	}

	// 2. 受信 (Pull) & 外部データ特定
	client := h.OAuthConfig.Client(context.Background(), &token)

	// 活動量（歩数・消費カロリー）の集計
	requestBody := map[string]interface{}{
		"aggregateBy": []map[string]interface{}{
			{"dataTypeName": "com.google.step_count.delta"},
			{"dataTypeName": "com.google.calories.expended"},
			{"dataTypeName": "com.google.nutrition"},
		},
		"bucketByTime":    map[string]interface{}{"durationMillis": 86400000},
		"startTimeMillis": startTime.UnixNano() / int64(time.Millisecond),
		"endTimeMillis":   endTime.UnixNano() / int64(time.Millisecond),
	}

	jsonReq, _ := json.Marshal(requestBody)
	resp, err := client.Post("https://www.googleapis.com/fitness/v1/users/me/dataset:aggregate", "application/json", bytes.NewBuffer(jsonReq))
	if err != nil {
		return c.Status(500).SendString("Google Fit APIへのリクエストに失敗しました")
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("Google Fit API Error: %s, Details: %s", resp.Status, string(body))
		return c.Status(resp.StatusCode).SendString("Google Fit APIエラー: " + string(body))
	}

	// Google Fitのレスポンス構造を解析するための構造体
	var fitData struct {
		Bucket []struct {
			Dataset []struct {
				Point []struct {
					Value []struct {
						IntVal int     `json:"intVal"`
						FpVal  float64 `json:"fpVal"`
					} `json:"value"`
				} `json:"point"`
			} `json:"dataset"`
		} `json:"bucket"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&fitData); err != nil {
		return c.Status(500).SendString("APIレスポンスの解析に失敗しました")
	}

	steps := 0
	calories := 0.0
	externalCalories := 0.0

	// 取得したバケットから歩数とカロリーを抽出
	if len(fitData.Bucket) > 0 {
		for _, ds := range fitData.Bucket[0].Dataset {
			for _, p := range ds.Point {
				for _, v := range p.Value {
					// 歩数は intVal、カロリーは fpVal に格納される
					if v.IntVal > 0 {
						steps += v.IntVal
					}
					if v.FpVal > 0 {
						calories += v.FpVal
					}
				}
			}
		}
	}

	// 栄養素の詳細比較（重複排除ロジック）
	rawNutritionURL := fmt.Sprintf("https://www.googleapis.com/fitness/v1/users/me/dataSources/derived:com.google.nutrition:com.google.android.gms:merged/datasets/%d-%d", startTime.UnixNano(), endTime.UnixNano())
	rawResp, err := client.Get(rawNutritionURL)
	if err == nil && rawResp.StatusCode == 200 {
		var rawData struct {
			Point []struct {
				StartTimeNanos     string `json:"startTimeNanos"`
				OriginDataSourceId string `json:"originDataSourceId"`
				Value              []struct {
					MapVal []struct {
						Key   string `json:"key"`
						Value struct {
							FpVal float64 `json:"fpVal"`
						} `json:"value"`
					} `json:"mapVal"`
				} `json:"value"`
			} `json:"point"`
		}
		json.NewDecoder(rawResp.Body).Decode(&rawData)
		rawResp.Body.Close()

		for _, p := range rawData.Point {
			pStart, _ := strconv.ParseInt(p.StartTimeNanos, 10, 64)
			isMyData := p.OriginDataSourceId == mySourceID || (mySourceID != "" && strings.Contains(p.OriginDataSourceId, "RecipeApp"))

			pCal := 0.0
			for _, v := range p.Value {
				for _, mv := range v.MapVal {
					if mv.Key == "calories" {
						pCal = mv.Value.FpVal
					}
				}
			}
			matched := false
			for idx, exp := range expectedMeals {
				if exp.startTimeNs == pStart && math.Abs(exp.calories-pCal) < 0.1 {
					expectedMeals[idx].foundInFit = true
					matched = true
					break
				}
			}
			if !matched && !isMyData && pCal > 0 {
				externalCalories += pCal
			}
		}
	}

	// 取得した活動データをDBに保存
	log.Printf("Fit Activity Pull: %d steps, %d kcal burned", steps, int(calories))

	// 3. 送信 (Push)：Fit に存在しなかった差分のみを送信
	for _, exp := range expectedMeals {
		if !exp.foundInFit {
			log.Printf("DEBUG: Sync target found - %s (%.2f kcal)", exp.mealTypeStr, exp.calories)

			// その食事区分の全レシピタイトルを取得して連結 (例: "トースト２枚, 目玉焼き")
			var titles []string // TODO: スプレッドシートから取得
			combinedTitle := strings.Join(titles, ", ")
			if combinedTitle == "" {
				combinedTitle = exp.mealTypeStr
			}
		}
	}

	return c.Redirect("/calendar?date=" + dateStr)
}
