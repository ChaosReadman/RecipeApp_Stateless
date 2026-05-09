package models

import (
	"database/sql"
)

type Food struct {
	ID   string
	Name string
}

// Search は名前で食品を検索します
func Search(db *sql.DB, query string) ([]Food, error) {
	var rows *sql.Rows
	var err error

	if query != "" {
		rows, err = db.Query("SELECT food_id, name FROM foods WHERE name LIKE ? LIMIT 50", "%"+query+"%")
	} else {
		rows, err = db.Query("SELECT food_id, name FROM foods LIMIT 20")
	}

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var foods []Food
	for rows.Next() {
		var f Food
		if err := rows.Scan(&f.ID, &f.Name); err != nil {
			return nil, err
		}
		foods = append(foods, f)
	}
	return foods, nil
}

// GetByID はIDから詳細情報を取得します（動的なマップを返します）
func GetByID(db *sql.DB, id string) (map[string]interface{}, error) {
	rows, err := db.Query("SELECT * FROM foods WHERE food_id = ?", id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	if !rows.Next() {
		return nil, nil
	}

	columns, _ := rows.Columns()
	values := make([]interface{}, len(columns))
	valuePtrs := make([]interface{}, len(columns))
	for i := range values {
		valuePtrs[i] = &values[i]
	}

	if err := rows.Scan(valuePtrs...); err != nil {
		return nil, err
	}

	foodDetail := make(map[string]interface{})
	for i, colName := range columns {
		val := values[i]
		if b, ok := val.([]byte); ok {
			foodDetail[colName] = string(b)
		} else {
			foodDetail[colName] = val
		}
	}
	return foodDetail, nil
}

// GetUserRecipeIngredients はユーザーがレシピで使用したことのある材料を取得します
func GetUserRecipeIngredients(db *sql.DB, userID int) ([]Food, error) {
	query := `
		SELECT DISTINCT f.food_id, f.name 
		FROM foods f 
		JOIN recipe_ingredients ri ON f.food_id = ri.food_id 
		JOIN recipes r ON ri.recipe_id = r.id 
		WHERE r.user_id = ?
		LIMIT 20`
	rows, err := db.Query(query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var foods []Food
	for rows.Next() {
		var f Food
		if err := rows.Scan(&f.ID, &f.Name); err != nil {
			return nil, err
		}
		foods = append(foods, f)
	}
	return foods, nil
}

// SearchRecipes はレシピを検索または一覧取得します
func SearchRecipes(db *sql.DB, query string) ([]map[string]interface{}, error) {
	var rows *sql.Rows
	var err error
	if query != "" {
		rows, err = db.Query("SELECT id, title, COALESCE(description, '') FROM recipes WHERE title LIKE ? OR description LIKE ? LIMIT 20", "%"+query+"%", "%"+query+"%")
	} else {
		rows, err = db.Query("SELECT id, title, COALESCE(description, '') FROM recipes ORDER BY created_at DESC LIMIT 10")
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var recipes []map[string]interface{}
	for rows.Next() {
		var id int
		var title, desc string
		rows.Scan(&id, &title, &desc)
		recipes = append(recipes, map[string]interface{}{"ID": id, "Title": title, "Description": desc})
	}
	return recipes, nil
}

// SearchRecipesScoped は範囲（マイレシピ/全レシピ）を指定して検索します
func SearchRecipesScoped(db *sql.DB, query string, userID int, scope string) ([]map[string]interface{}, error) {
	var rows *sql.Rows
	var err error

	sql := "SELECT id, title, COALESCE(description, '') FROM recipes WHERE (title LIKE ? OR description LIKE ?)"
	args := []interface{}{"%" + query + "%", "%" + query + "%"}

	if scope == "my" {
		sql += " AND user_id = ?"
		args = append(args, userID)
	}

	sql += " ORDER BY created_at DESC LIMIT 20"

	rows, err = db.Query(sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var recipes []map[string]interface{}
	for rows.Next() {
		var id int
		var title, desc string
		rows.Scan(&id, &title, &desc)
		recipes = append(recipes, map[string]interface{}{"ID": id, "Title": title, "Description": desc})
	}
	return recipes, nil
}

type RecipeIngredientDetail struct {
	FoodID    string
	Name      string
	Quantity  float64
	GroupName string
}

type RecipeStepDetail struct {
	StepNumber  int
	Instruction string
}

type RecipeFull struct {
	ID            int
	UserID        int
	Title         string
	Description   string
	Ingredients   []RecipeIngredientDetail
	Steps         []RecipeStepDetail
	TotalCalories float64
	EntryTime     string
	TotalProtein  float64
	TotalFat      float64
	TotalCarbs    float64
	TotalFiber    float64
	TotalSodium   float64
}

// GetRecipeByID はレシピの全情報を取得します
func GetRecipeByID(db *sql.DB, id string) (*RecipeFull, error) {
	var r RecipeFull
	err := db.QueryRow("SELECT id, user_id, title, COALESCE(description, '') FROM recipes WHERE id = ?", id).Scan(&r.ID, &r.UserID, &r.Title, &r.Description)
	if err == sql.ErrNoRows {
		return nil, nil
	} else if err != nil {
		return nil, err
	}

	// 材料の取得 (foodsテーブルと結合して名称を取得)
	rows, err := db.Query(`
		SELECT f.food_id, f.name, ri.quantity, COALESCE(ri.group_name, ''),
		       COALESCE(f.enerc_kcal, 0), COALESCE(f.prot_, 0), COALESCE(f.fat_, 0), 
		       COALESCE(f.chocdf_, 0)
		FROM recipe_ingredients ri 
		JOIN foods f ON ri.food_id = f.food_id 
		WHERE ri.recipe_id = ?`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var ing RecipeIngredientDetail
		var kcal, prot, fat, carb float64
		rows.Scan(&ing.FoodID, &ing.Name, &ing.Quantity, &ing.GroupName, &kcal, &prot, &fat, &carb)
		r.Ingredients = append(r.Ingredients, ing)

		// 合計栄養素の計算 (食品データは100gあたりの値)
		r.TotalCalories += (kcal * ing.Quantity / 100.0)
		r.TotalProtein += (prot * ing.Quantity / 100.0)
		r.TotalFat += (fat * ing.Quantity / 100.0)
		r.TotalCarbs += (carb * ing.Quantity / 100.0)
	}

	// 工程の取得
	rows, err = db.Query("SELECT step_number, COALESCE(instruction, '') FROM recipe_steps WHERE recipe_id = ? ORDER BY step_number ASC", id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var step RecipeStepDetail
		rows.Scan(&step.StepNumber, &step.Instruction)
		r.Steps = append(r.Steps, step)
	}

	return &r, nil
}

type CalendarEntryDetail struct {
	ID        int
	RecipeID  int
	Title     string
	MealType  string
	EntryDate string
	EntryTime string
	IsSynced  bool
}

// GetCalendarEntries は指定したユーザーと日付の食事記録を取得します
func GetCalendarEntries(db *sql.DB, userID int, date string) ([]CalendarEntryDetail, error) {
	query := `
		SELECT ce.id, ce.recipe_id, r.title, ce.meal_type, DATE(ce.entry_date), COALESCE(ce.entry_time, '00:00'), ce.is_synced
		FROM calendar_entries ce
		JOIN recipes r ON ce.recipe_id = r.id
		WHERE ce.user_id = ? AND date(ce.entry_date) = ?
		ORDER BY CASE ce.meal_type 
			WHEN 'breakfast' THEN 1 
			WHEN 'lunch' THEN 2 
			WHEN 'dinner' THEN 3 
			ELSE 4 END`

	rows, err := db.Query(query, userID, date)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []CalendarEntryDetail
	for rows.Next() {
		var e CalendarEntryDetail
		rows.Scan(&e.ID, &e.RecipeID, &e.Title, &e.MealType, &e.EntryDate, &e.EntryTime, &e.IsSynced)
		entries = append(entries, e)
	}
	return entries, nil
}

// GetUserRecipes はユーザーが作成したレシピ一覧を取得します
func GetUserRecipes(db *sql.DB, userID int) ([]map[string]interface{}, error) {
	rows, err := db.Query("SELECT id, title FROM recipes WHERE user_id = ? ORDER BY created_at DESC", userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var recipes []map[string]interface{}
	for rows.Next() {
		var id int
		var title string
		rows.Scan(&id, &title)
		recipes = append(recipes, map[string]interface{}{"ID": id, "Title": title})
	}
	return recipes, nil
}

// GetDailyCalories は指定したユーザーと日付の合計摂取カロリーを計算します
func GetDailyCalories(db *sql.DB, userID int, date string) (float64, int, error) {
	query := `
		SELECT SUM(f.enerc_kcal * ri.quantity / 100.0)
		FROM calendar_entries ce
		JOIN recipe_ingredients ri ON ce.recipe_id = ri.recipe_id
		JOIN foods f ON ri.food_id = f.food_id
		WHERE ce.user_id = ? AND date(ce.entry_date) = ?`

	var managedTotal sql.NullFloat64
	err := db.QueryRow(query, userID, date).Scan(&managedTotal)
	if err != nil {
		return 0, 0, err
	}

	var external int
	db.QueryRow("SELECT external_intake_calories FROM daily_health_data WHERE user_id = ? AND date(date) = ?", userID, date).Scan(&external)

	return managedTotal.Float64, external, nil
}

// GetDailyHealthData は指定したユーザーと日付の歩数と消費カロリーを取得します
func GetDailyHealthData(db *sql.DB, userID int, date string) (int, int, bool) {
	var steps, calories int
	var isSynced bool
	query := "SELECT steps, burned_calories, is_synced FROM daily_health_data WHERE user_id = ? AND date(date) = ?"
	err := db.QueryRow(query, userID, date).Scan(&steps, &calories, &isSynced)
	if err != nil {
		// データがない場合は 0 を返す
		return 0, 0, false
	}
	return steps, calories, isSynced
}

// GetMealTypeNutrition は指定したユーザー、日付、食事区分の合計栄養素を取得します
func GetMealTypeNutrition(db *sql.DB, userID int, date, mealType string) (*RecipeFull, error) {
	var totalNutrition RecipeFull
	totalNutrition.Title = mealType

	query := `
		SELECT
			MAX(COALESCE(ce.entry_time, '00:00')),
			SUM(COALESCE(f.enerc_kcal, 0) * ri.quantity / 100.0),
			SUM(COALESCE(f.prot_, 0) * ri.quantity / 100.0),
			SUM(COALESCE(f.fat_, 0) * ri.quantity / 100.0),
			SUM(COALESCE(f.chocdf_, 0) * ri.quantity / 100.0)
		FROM calendar_entries ce
		JOIN recipe_ingredients ri ON ce.recipe_id = ri.recipe_id
		JOIN foods f ON ri.food_id = f.food_id
		WHERE ce.user_id = ? AND date(ce.entry_date) = ? AND ce.meal_type = ?`

	var totalCalories, totalProtein, totalFat, totalCarbs sql.NullFloat64
	var entryTime string

	err := db.QueryRow(query, userID, date, mealType).Scan(
		&entryTime, &totalCalories, &totalProtein, &totalFat, &totalCarbs,
	)
	if err != nil {
		return nil, err
	}

	totalNutrition.EntryTime = entryTime
	totalNutrition.TotalCalories = totalCalories.Float64
	totalNutrition.TotalProtein = totalProtein.Float64
	totalNutrition.TotalFat = totalFat.Float64
	totalNutrition.TotalCarbs = totalCarbs.Float64

	return &totalNutrition, nil
}
