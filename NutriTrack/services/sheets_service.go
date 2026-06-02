package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
)

const (
	MealHistoryFile = "Calendar.json"
)

var mu sync.Mutex

// getOrCreateFolder は Google Drive 上に指定されたパスのフォルダ階層を探し、なければ作成して最下層のフォルダの ID を返します
// 例: "NutriTrack\\Recipes\\MyGroup" のようなパスを受け取り、各階層のフォルダを作成/取得し、最終的に "MyGroup" フォルダのIDを返します。
func getOrCreateFolder(srv *drive.Service, folderPath string) (string, error) {
	// パスセパレータを正規化 (Windowsの\とUnix系の/の両方に対応)
	folderPath = strings.ReplaceAll(folderPath, "\\", "/")
	parts := strings.Split(folderPath, "/")
	if len(parts) == 0 {
		return "", errors.New("empty folder path provided")
	}

	currentParentID := "root" // Google Driveのルートから開始

	for _, part := range parts {
		if part == "" {
			continue // 空のパスセグメントはスキップ (例: "//" や末尾の "/")
		}

		// 現在の親フォルダ内で、指定された名前のフォルダを検索
		query := fmt.Sprintf("name='%s' and mimeType='application/vnd.google-apps.folder' and '%s' in parents and trashed=false", part, currentParentID)
		list, err := srv.Files.List().Q(query).Do()
		if err != nil {
			return "", fmt.Errorf("failed to list files for folder '%s' in parent '%s': %w", part, currentParentID, err)
		}

		if len(list.Files) > 0 {
			// フォルダが見つかった場合、そのIDを次の親IDとする
			currentParentID = list.Files[0].Id
		} else {
			// フォルダが見つからなかった場合、作成する
			folder := &drive.File{
				Name:     part,
				MimeType: "application/vnd.google-apps.folder",
				Parents:  []string{currentParentID}, // 現在の親IDを指定
			}
			res, err := srv.Files.Create(folder).Do()
			if err != nil {
				return "", fmt.Errorf("failed to create folder '%s' in parent '%s': %w", part, currentParentID, err)
			}
			currentParentID = res.Id
		}
	}
	return currentParentID, nil
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
func CreateRecipeJSON(ctx context.Context, client *http.Client, title, group, description string, ingredients []map[string]interface{}, steps []string) error {
	mu.Lock()
	defer mu.Unlock()

	srv, err := drive.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return err
	}

	// レシピファイルを保存する親フォルダ (NutriTrack/Recipes) を取得
	recipesFolderID, err := getOrCreateFolder(srv, "NutriTrack\\Recipes")
	if err != nil {
		return err
	}
	fileID, err := getOrCreateFile(srv, recipesFolderID, group+".json")
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

func DeleteRecipe(ctx context.Context, client *http.Client, id, group string) error {
	mu.Lock()
	defer mu.Unlock()

	srv, err := drive.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return err
	}

	// レシピファイルを保存する親フォルダ (NutriTrack/Recipes) を取得
	recipesFolderID, err := getOrCreateFolder(srv, "NutriTrack\\Recipes")
	if err != nil {
		return err
	}
	fileID, err := getOrCreateFile(srv, recipesFolderID, group+".json")
	if err != nil {
		return err
	}

	var recipes []map[string]interface{}
	if err := downloadJSON(srv, fileID, &recipes); err != nil {
		return fmt.Errorf("failed to download recipes: %w", err)
	}

	// 指定されたID以外のレシピだけを残す（フィルタリング）
	newRecipes := []map[string]interface{}{}
	targetID := strings.TrimSpace(id)
	for _, r := range recipes {
		currentID := strings.TrimSpace(fmt.Sprintf("%v", r["ID"]))
		if currentID == targetID {
			continue // 削除対象
		}
		newRecipes = append(newRecipes, r)
	}

	if len(recipes) == len(newRecipes) {
		return errors.New("recipe not found")
	}

	return uploadJSON(srv, fileID, newRecipes)
}

// UpdateRecipe は既存のレシピを更新します
func UpdateRecipe(ctx context.Context, client *http.Client, id, group, title, description string, ingredients []map[string]interface{}, steps []string) error {
	mu.Lock()
	defer mu.Unlock()

	srv, err := drive.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return err
	}

	// レシピファイルを保存する親フォルダ (NutriTrack/Recipes) を取得
	recipesFolderID, err := getOrCreateFolder(srv, "NutriTrack\\Recipes")
	if err != nil {
		return err
	}
	fileID, err := getOrCreateFile(srv, recipesFolderID, group+".json")
	if err != nil {
		return err
	}

	var recipes []map[string]interface{}
	if err := downloadJSON(srv, fileID, &recipes); err != nil {
		return fmt.Errorf("failed to download recipes: %w", err)
	}

	// 工程（Steps）の変換
	formattedSteps := make([]map[string]interface{}, len(steps))
	for i, step := range steps {
		formattedSteps[i] = map[string]interface{}{
			"StepNumber":  i + 1,
			"Instruction": step,
		}
	}

	found := false
	targetID := strings.TrimSpace(id)
	for i, r := range recipes {
		currentID := strings.TrimSpace(fmt.Sprintf("%v", r["ID"]))
		if currentID == targetID {
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

// FetchRecipes は recipesフォルダの json  からレシピ一覧を取得します
func FetchRecipes(ctx context.Context, client *http.Client, query string) ([]map[string]interface{}, error) {
	if client == nil {
		return nil, errors.New("client is nil")
	}
	mu.Lock()
	defer mu.Unlock()

	srv, err := drive.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return nil, err
	}

	// "NutriTrack/Recipes" フォルダのIDを取得
	recipesFolderID, err := getOrCreateFolder(srv, "NutriTrack\\Recipes")
	if err != nil {
		return nil, err
	}

	fileList, err := listFiles(srv, recipesFolderID)
	if err != nil {
		return nil, err
	}

	var allRecipes []map[string]interface{}
	// recipesFolderID内のすべての.jsonファイルを反復処理
	for _, file := range fileList.Files {
		if !strings.HasSuffix(file.Name, ".json") {
			continue // .jsonファイル以外はスキップ
		}
		var groupRecipes []map[string]interface{}
		if err := downloadJSON(srv, file.Id, &groupRecipes); err != nil {
			// エラーをログに記録し、他のファイルで続行
			log.Printf("Warning: Failed to download/parse recipe file %s (%s): %v\n", file.Name, file.Id, err)
			continue
		}

		// ファイル名から .json を除いたものをグループ名とする
		Group := strings.TrimSuffix(file.Name, ".json")
		// 取得したレシピすべてにグループ名を付与
		for i := range groupRecipes {
			groupRecipes[i]["Group"] = Group
		}
		allRecipes = append(allRecipes, groupRecipes...)
	}

	if query == "" {
		return allRecipes, nil
	}

	var filtered []map[string]interface{}
	for _, r := range allRecipes {
		title, _ := r["Title"].(string)
		if strings.Contains(strings.ToLower(title), strings.ToLower(query)) {
			filtered = append(filtered, r)
		}
	}
	return filtered, nil
}

func listFiles(srv *drive.Service, folderID string) (*drive.FileList, error) {
	// listFilesの戻り値の型をdrive.FileListに明示
	return drive.NewFilesService(srv).List().Q(fmt.Sprintf("'%s' in parents and trashed=false", folderID)).Do()
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

// ListRecipeGroups は NutriTrack/Recipes フォルダ内の JSON ファイル名（拡張子なし）を一覧で取得します
func ListRecipeGroups(ctx context.Context, client *http.Client) ([]string, error) {
	mu.Lock()
	defer mu.Unlock()

	srv, err := drive.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return nil, err
	}

	folderID, err := getOrCreateFolder(srv, "NutriTrack\\Recipes")
	if err != nil {
		return nil, err
	}

	fileList, err := listFiles(srv, folderID)
	if err != nil {
		return nil, err
	}

	var groups []string
	for _, file := range fileList.Files {
		if strings.HasSuffix(file.Name, ".json") {
			groups = append(groups, strings.TrimSuffix(file.Name, ".json"))
		}
	}
	return groups, nil
}
