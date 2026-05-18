package models

import (
	"database/sql"
)

type User struct {
	ID              int
	Provider        string // 認証プロバイダ (e.g., "google")
	ExternalID      string // プロバイダ固有のユーザーID (e.g., Google ID)
	FitDataSourceID string // Google Fit のデータソースID
}

// FindOrCreate はOAuth情報に基づいてユーザーを取得または新規作成します
func FindOrCreate(db *sql.DB, provider, externalID string) (*User, error) {
	var user User
	// external_id と provider でユーザーを検索
	err := db.QueryRow("SELECT id, provider, external_id, COALESCE(fit_data_source_id, '') FROM users WHERE provider = ? AND external_id = ?", provider, externalID).
		Scan(&user.ID, &user.Provider, &user.ExternalID, &user.FitDataSourceID)

	if err == sql.ErrNoRows {
		// 新規ユーザー作成
		res, err := db.Exec(
			"INSERT INTO users (provider, external_id) VALUES (?, ?)",
			provider, externalID,
		)
		if err != nil {
			return nil, err
		}
		id, _ := res.LastInsertId()
		return &User{ID: int(id), Provider: provider, ExternalID: externalID, FitDataSourceID: ""}, nil
	} else if err != nil {
		return nil, err
	}

	return &user, nil
}

// GetUserByID はIDからユーザー情報を取得します
func GetUserByID(db *sql.DB, id int) (*User, error) {
	var user User
	// email, name はDBに保存しないため取得しない
	err := db.QueryRow("SELECT id, provider, external_id, fit_data_source_id FROM users WHERE id = ?", id).Scan(&user.ID, &user.Provider, &user.ExternalID, &user.FitDataSourceID)
	if err != nil {
		return nil, err
	}
	return &user, nil
}
