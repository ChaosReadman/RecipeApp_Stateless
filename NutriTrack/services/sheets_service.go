package services

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"

	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"
)

const (
	RecipeSpreadsheetName = "NutriTrack_Recipes"
	RecipeSheetName       = "Recipes"
)

// GetOrCreateRecipeSpreadsheet は指定された名前のスプレッドシートを検索し、
// 存在しない場合は新規作成してそのIDを返します。
// また、そのスプレッドシート内に "Recipes" という名前のシートが存在することを確認し、
// なければ作成します。
func GetOrCreateRecipeSpreadsheet(ctx context.Context, client *http.Client) (string, error) {
	driveSrv, err := drive.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return "", fmt.Errorf("Google Drive APIの初期化に失敗しました: %w", err)
	}

	// 既存のスプレッドシートを検索
	// mimeType='application/vnd.google-apps.spreadsheet' でスプレッドシートに限定
	// trashed=false でゴミ箱にないものに限定
	query := fmt.Sprintf("name='%s' and mimeType='application/vnd.google-apps.spreadsheet' and trashed=false", RecipeSpreadsheetName)
	r, err := driveSrv.Files.List().Q(query).Fields("files(id, name)").Do()
	if err != nil {
		return "", fmt.Errorf("スプレッドシートの検索に失敗しました: %w", err)
	}

	var spreadsheetId string
	if len(r.Files) > 0 {
		// 既存のスプレッドシートが見つかった場合
		spreadsheetId = r.Files[0].Id
		log.Printf("既存のスプレッドシート '%s' を見つけました: %s", RecipeSpreadsheetName, spreadsheetId)
	} else {
		// 見つからなかった場合、新規作成
		log.Printf("スプレッドシート '%s' が見つかりませんでした。新規作成します。", RecipeSpreadsheetName)
		newSpreadsheet := &drive.File{
			Name:     RecipeSpreadsheetName,
			MimeType: "application/vnd.google-apps.spreadsheet",
		}
		file, err := driveSrv.Files.Create(newSpreadsheet).Fields("id").Do()
		if err != nil {
			return "", fmt.Errorf("スプレッドシートの新規作成に失敗しました: %w", err)
		}
		spreadsheetId = file.Id
		log.Printf("スプレッドシート '%s' を新規作成しました: %s", RecipeSpreadsheetName, spreadsheetId)
	}
	return spreadsheetId, nil
}

// CreateRecipeSheet はレシピ名で新しいシートを作成し、詳細情報を書き込みます
func CreateRecipeSheet(ctx context.Context, client *http.Client, spreadsheetId, title, description string, ingredients []map[string]interface{}, steps []string) error {
	srv, err := sheets.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return err
	}

	// 1. 既存のシートをチェック
	spreadsheet, err := srv.Spreadsheets.Get(spreadsheetId).Context(ctx).Do()
	if err != nil {
		return err
	}

	sheetExists := false
	for _, s := range spreadsheet.Sheets {
		if s.Properties.Title == title {
			sheetExists = true
			break
		}
	}

	// 2. シートが存在しない場合のみ新規作成
	if !sheetExists {
		addSheetReq := &sheets.BatchUpdateSpreadsheetRequest{
			Requests: []*sheets.Request{
				{
					AddSheet: &sheets.AddSheetRequest{
						Properties: &sheets.SheetProperties{Title: title},
					},
				},
			},
		}
		if _, err = srv.Spreadsheets.BatchUpdate(spreadsheetId, addSheetReq).Context(ctx).Do(); err != nil {
			return fmt.Errorf("シートの作成に失敗しました: %w", err)
		}
	}

	// 3. データの構成
	var data [][]interface{}
	data = append(data, []interface{}{"レシピ名", title}) // A1セルが「レシピ名」になります
	data = append(data, []interface{}{"説明", description})
	data = append(data, []interface{}{"作成日", fmt.Sprintf("%s", ctx.Value("timestamp"))})
	data = append(data, []interface{}{}) // 空行

	data = append(data, []interface{}{"【材料リスト】"})
	data = append(data, []interface{}{"食品ID", "名称", "重量(g)", "グループ"})
	for _, ing := range ingredients {
		data = append(data, []interface{}{ing["ID"], ing["Name"], ing["Weight"], ing["Group"]})
	}
	data = append(data, []interface{}{}) // 空行

	data = append(data, []interface{}{"【調理手順】"})
	data = append(data, []interface{}{"番号", "内容"})
	for i, step := range steps {
		data = append(data, []interface{}{i + 1, step})
	}

	// 3. 値の書き込み
	rb := &sheets.ValueRange{Values: data}
	_, err = srv.Spreadsheets.Values.Update(spreadsheetId, title+"!A1", rb).ValueInputOption("RAW").Context(ctx).Do()
	return err
}

// FetchRecipes はスプレッドシートの全シート名をレシピ一覧として取得します
func FetchRecipes(ctx context.Context, client *http.Client, spreadsheetId, query string) ([]map[string]interface{}, error) {
	srv, err := sheets.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return nil, err
	}

	spreadsheet, err := srv.Spreadsheets.Get(spreadsheetId).Context(ctx).Do()
	if err != nil {
		return nil, err
	}

	var recipes []map[string]interface{}
	for _, sheet := range spreadsheet.Sheets {
		name := sheet.Properties.Title
		// デフォルトのシートなどは除外
		if name == "シート1" || name == "Sheet1" {
			continue
		}

		// 検索クエリがある場合、シート名に含まれているかチェック（大文字小文字を区別しない）
		if query != "" && !strings.Contains(strings.ToLower(name), strings.ToLower(query)) {
			continue
		}

		recipes = append(recipes, map[string]interface{}{
			"ID":          name, // シート名をIDとして扱う
			"Title":       name,
			"Description": "スプレッドシートに保存済み",
		})
	}

	return recipes, nil
}
