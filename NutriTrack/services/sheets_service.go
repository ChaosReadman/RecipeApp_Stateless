package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
)

const (
	RecipesFile     = "recipes.json"
	MealHistoryFile = "Calendar.json"
)

var mu sync.Mutex

// GetOrCreateRecipeSpreadsheet は互換性のために残されています。
// 現在はスプレッドシートではなく Google Drive 上の JSON を使用しているため、ダミーの ID を返します。
func GetOrCreateRecipeSpreadsheet(ctx context.Context, client *http.Client) (string, error) {
	return "google_drive_json_mode", nil
}

// getOrCreateFolder は Google Drive 上に指定された名前のフォルダを探し、なければ作成して ID を返します
func getOrCreateFolder(srv *drive.Service, folderName string) (string, error) {
	query := fmt.Sprintf("name='%s' and mimeType='application/vnd.google-apps.folder' and trashed=false", folderName)
	list, err := srv.Files.List().Q(query).Do()
	if err != nil {
		return "", err
	}
	if len(list.Files) > 0 {
		return list.Files[0].Id, nil
	}
	folder := &drive.File{
		Name:     folderName,
		MimeType: "application/vnd.google-apps.folder",
	}
	res, err := srv.Files.Create(folder).Do()
	if err != nil {
		return "", err
	}
	return res.Id, nil
}

// getOrCreateFile は指定されたフォルダ内のファイルを探し、なければ空の JSON 配列で作成します
func getOrCreateFile(srv *drive.Service, folderID, fileName string) (string, error) {
	query := fmt.Sprintf("name='%s' and '%s' in parents and trashed=false", fileName, folderID)
	list, err := srv.Files.List().Q(query).Do()
	if err != nil {
		return "", err
	}
	if len(list.Files) > 0 {
		return list.Files[0].Id, nil
	}
	file := &drive.File{
		Name:     fileName,
		Parents:  []string{folderID},
		MimeType: "application/json",
	}
	res, err := srv.Files.Create(file).Media(strings.NewReader("[]")).Do()
	if err != nil {
		return "", err
	}
	return res.Id, nil
}

// downloadJSON は Google Drive から JSON ファイルをダウンロードしてデコードします
func downloadJSON(srv *drive.Service, fileID string, target interface{}) error {
	resp, err := srv.Files.Get(fileID).Download()
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, target)
}

// uploadJSON はデータを JSON 化して Google Drive 上のファイルを更新します
func uploadJSON(srv *drive.Service, fileID string, data interface{}) error {
	content, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	_, err = srv.Files.Update(fileID, &drive.File{}).Media(strings.NewReader(string(content))).Do()
	return err
}

// CreateRecipeSheet はレシピ情報を recipes.json に保存します
func CreateRecipeSheet(ctx context.Context, client *http.Client, spreadsheetId, title, description string, ingredients []map[string]interface{}, steps []string) error {
	mu.Lock()
	defer mu.Unlock()

	srv, err := drive.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return err
	}

	folderID, err := getOrCreateFolder(srv, "NutriTrack")
	if err != nil {
		return err
	}

	fileID, err := getOrCreateFile(srv, folderID, RecipesFile)
	if err != nil {
		return err
	}

	var recipes []map[string]interface{}
	downloadJSON(srv, fileID, &recipes)

	// 工程（Steps）をテンプレートが期待する形式 {StepNumber, Instruction} に変換
	formattedSteps := make([]map[string]interface{}, len(steps))
	for i, step := range steps {
		formattedSteps[i] = map[string]interface{}{
			"StepNumber":  i + 1,
			"Instruction": step,
		}
	}

	newRecipe := map[string]interface{}{
		"ID":          fmt.Sprintf("REC-%d", time.Now().Unix()),
		"Title":       title,
		"Description": description,
		"Ingredients": ingredients,
		"Steps":       formattedSteps,
		"CreatedAt":   time.Now().Format(time.RFC3339),
	}

	recipes = append(recipes, newRecipe)
	return uploadJSON(srv, fileID, recipes)
}

// UpdateRecipe は既存のレシピを更新します
func UpdateRecipe(ctx context.Context, client *http.Client, id, title, description string, ingredients []map[string]interface{}, steps []string) error {
	mu.Lock()
	defer mu.Unlock()

	srv, err := drive.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return err
	}

	folderID, err := getOrCreateFolder(srv, "NutriTrack")
	if err != nil {
		return err
	}

	fileID, err := getOrCreateFile(srv, folderID, RecipesFile)
	if err != nil {
		return err
	}

	var recipes []map[string]interface{}
	downloadJSON(srv, fileID, &recipes)

	// 工程（Steps）の変換
	formattedSteps := make([]map[string]interface{}, len(steps))
	for i, step := range steps {
		formattedSteps[i] = map[string]interface{}{
			"StepNumber":  i + 1,
			"Instruction": step,
		}
	}

	found := false
	for i, r := range recipes {
		if fmt.Sprintf("%v", r["ID"]) == id {
			recipes[i]["Title"] = title
			recipes[i]["Description"] = description
			recipes[i]["Ingredients"] = ingredients
			recipes[i]["Steps"] = formattedSteps
			recipes[i]["UpdatedAt"] = time.Now().Format(time.RFC3339)
			found = true
			break
		}
	}

	if !found {
		return errors.New("recipe not found")
	}

	return uploadJSON(srv, fileID, recipes)
}

// FetchRecipes は recipes.json からレシピ一覧を取得します
func FetchRecipes(ctx context.Context, client *http.Client, spreadsheetId, query string) ([]map[string]interface{}, error) {
	if client == nil {
		return nil, errors.New("client is nil")
	}
	mu.Lock()
	defer mu.Unlock()

	srv, err := drive.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return nil, err
	}

	folderID, err := getOrCreateFolder(srv, "NutriTrack")
	if err != nil {
		return nil, err
	}

	fileID, err := getOrCreateFile(srv, folderID, RecipesFile)
	if err != nil {
		return nil, err
	}

	var recipes []map[string]interface{}
	if err := downloadJSON(srv, fileID, &recipes); err != nil {
		return nil, err
	}

	if query == "" {
		return recipes, nil
	}

	var filtered []map[string]interface{}
	for _, r := range recipes {
		title, _ := r["Title"].(string)
		if strings.Contains(strings.ToLower(title), strings.ToLower(query)) {
			filtered = append(filtered, r)
		}
	}
	return filtered, nil
}

// FetchMealHistory は Calendar.json から全履歴を取得します
func FetchMealHistory(ctx context.Context, client *http.Client) ([]map[string]interface{}, error) {
	if client == nil {
		return nil, errors.New("client is nil")
	}
	mu.Lock()
	defer mu.Unlock()

	srv, err := drive.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return nil, err
	}

	folderID, err := getOrCreateFolder(srv, "NutriTrack")
	if err != nil {
		return nil, err
	}

	fileID, err := getOrCreateFile(srv, folderID, MealHistoryFile)
	if err != nil {
		return nil, err
	}

	var history []map[string]interface{}
	if err := downloadJSON(srv, fileID, &history); err != nil {
		// ファイルが空の場合は空配列を返す
		if strings.Contains(err.Error(), "unexpected end of JSON input") {
			return []map[string]interface{}{}, nil
		}
		return nil, err
	}

	return history, nil
}

// SaveMealHistory は食事記録を meal_history.json に保存します
func SaveMealHistory(ctx context.Context, client *http.Client, record map[string]interface{}) error {
	mu.Lock()
	defer mu.Unlock()

	srv, err := drive.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return err
	}

	folderID, err := getOrCreateFolder(srv, "NutriTrack")
	if err != nil {
		return err
	}

	fileID, err := getOrCreateFile(srv, folderID, MealHistoryFile)
	if err != nil {
		return err
	}

	var history []map[string]interface{}
	downloadJSON(srv, fileID, &history)

	// 指定されたTimestampがない場合のみ、現在の時刻をセットする
	if _, exists := record["Timestamp"]; !exists {
		record["Timestamp"] = time.Now().Format(time.RFC3339)
	}
	history = append(history, record)

	return uploadJSON(srv, fileID, history)
}

// RemoveMealHistory は Calendar.json から特定のタイムスタンプの記録を削除します
func RemoveMealHistory(ctx context.Context, client *http.Client, timestamp string) error {
	mu.Lock()
	defer mu.Unlock()

	srv, err := drive.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return err
	}

	folderID, err := getOrCreateFolder(srv, "NutriTrack")
	if err != nil {
		return err
	}

	fileID, err := getOrCreateFile(srv, folderID, MealHistoryFile)
	if err != nil {
		return err
	}

	var history []map[string]interface{}
	if err := downloadJSON(srv, fileID, &history); err != nil {
		return err
	}

	// タイムスタンプが一致しないものだけを残す（フィルタリング）
	newHistory := []map[string]interface{}{}
	for _, record := range history {
		if ts, ok := record["Timestamp"].(string); ok && ts == timestamp {
			continue // 削除対象
		}
		newHistory = append(newHistory, record)
	}

	if len(history) == len(newHistory) {
		return errors.New("record not found")
	}

	return uploadJSON(srv, fileID, newHistory)
}
