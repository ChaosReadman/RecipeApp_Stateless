package models

import (
	"database/sql"
)

type User struct {
	ID              int
	Email           string
	Name            string
	FitDataSourceID string // Google Fit のデータソースID
}

// FindOrCreate はOAuth情報に基づいてユーザーを取得または新規作成します
func FindOrCreate(db *sql.DB, email, name, provider, providerID string) (*User, error) {
	var user User
	err := db.QueryRow("SELECT id, email, name FROM users WHERE email = ?", email).Scan(&user.ID, &user.Email, &user.Name)

	if err == sql.ErrNoRows {
		// 新規ユーザー作成
		res, err := db.Exec(
			"INSERT INTO users (email, name, provider, provider_id) VALUES (?, ?, ?, ?)",
			email, name, provider, providerID,
		)
		if err != nil {
			return nil, err
		}
		id, _ := res.LastInsertId()
		return &User{ID: int(id), Email: email, Name: name}, nil
	} else if err != nil {
		return nil, err
	}

	return &user, nil
}

// GetUserByID はIDからユーザー情報を取得します
func GetUserByID(db *sql.DB, id int) (*User, error) {
	var user User
	err := db.QueryRow("SELECT id, email, name, fit_data_source_id FROM users WHERE id = ?", id).Scan(&user.ID, &user.Email, &user.Name, &user.FitDataSourceID)
	if err != nil {
		return nil, err
	}
	return &user, nil
}
