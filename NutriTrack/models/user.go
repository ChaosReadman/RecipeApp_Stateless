package models

// User はアプリケーション内で利用するユーザー識別情報です
type User struct {
	ID              int
	Provider        string // 認証プロバイダ (e.g., "google")
	ExternalID      string // プロバイダ固有のユーザーID (e.g., Google ID)
	FitDataSourceID string // Google Fit のデータソースID
}
