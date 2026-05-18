package handlers

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"log"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/session"
	"golang.org/x/oauth2"
)

type AuthHandler struct {
	Store       *session.Store
	OAuthConfig *oauth2.Config
}

// ShowLogin はログイン画面を表示します
func (h *AuthHandler) ShowLogin(c *fiber.Ctx) error {
	return c.Render("login", fiber.Map{
		"Title":                "ログイン",
		"HideIngredientDrawer": true,
	}, "") // ログイン画面には共通レイアウトを適用しない
}

// Login リダイレクト処理
func (h *AuthHandler) Login(c *fiber.Ctx) error {
	// CSRF対策のため、ランダムな文字列(state)を生成してセッションに保存します
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return c.Status(500).SendString("内部エラーが発生しました")
	}
	state := base64.URLEncoding.EncodeToString(b)

	sess, err := h.Store.Get(c)
	if err != nil {
		return c.Status(500).SendString("セッションの取得に失敗しました")
	}
	sess.Set("oauth_state", state)
	if err := sess.Save(); err != nil {
		return c.Status(500).SendString("セッションの保存に失敗しました")
	}

	// ApprovalForce を削除することで、二回目以降のログインがスムーズになります
	url := h.OAuthConfig.AuthCodeURL(state, oauth2.AccessTypeOffline)
	return c.Redirect(url)
}

// Callback OAuthからの戻り先
func (h *AuthHandler) Callback(c *fiber.Ctx) error {
	sess, err := h.Store.Get(c)
	if err != nil {
		return c.Status(500).SendString("セッションの取得に失敗しました")
	}

	// CSRF対策: Stateの検証
	code := c.Query("code")
	state := c.Query("state")
	savedState := sess.Get("oauth_state")

	if savedState == nil {
		log.Println("[AUTH ERROR] Session state is nil. Cookie might be blocked or lost.")
		return c.Status(400).SendString("不正なリクエストです (セッションが失われました)")
	}
	if state != savedState.(string) {
		log.Printf("[AUTH ERROR] State mismatch!\n  Expected (Session): %v\n  Got (URL): %v", savedState, state)
		// 不一致が起きた場合は、古いStateを削除して再試行を促す
		sess.Delete("oauth_state")
		sess.Save()
		return c.Status(400).SendString("不正なリクエストです (State不一致)")
	}
	sess.Delete("oauth_state")

	token, err := h.OAuthConfig.Exchange(context.Background(), code)
	if err != nil {
		return c.Status(500).SendString("トークンの取得に失敗しました")
	}

	// Googleからユーザー情報を取得
	client := h.OAuthConfig.Client(context.Background(), token)
	resp, err := client.Get("https://www.googleapis.com/oauth2/v2/userinfo")
	if err != nil {
		return c.Status(500).SendString("ユーザー情報の取得に失敗しました")
	}
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusOK {
		return c.Status(resp.StatusCode).SendString("Google APIからのデータ取得に失敗しました (Status: " + resp.Status + ")")
	}

	var profile struct {
		ID    string `json:"id"`
		Email string `json:"email"`
		Name  string `json:"name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&profile); err != nil {
		return c.Status(500).SendString("プロフィールの解析に失敗しました")
	}

	tokenData, _ := json.Marshal(token)

	sess.Set("user_id", profile.ID)    // GoogleのID（文字列）をそのまま識別子として使用
	sess.Set("username", profile.Name) // ユーザー名はGoogleから取得したものを直接セッションに保存
	sess.Set("oauth_token", string(tokenData))
	if err := sess.Save(); err != nil {
		return c.Status(500).SendString("ログイン情報の保存に失敗しました")
	}

	return c.Redirect("/")
}

// Logout はログアウト処理を実行します
func (h *AuthHandler) Logout(c *fiber.Ctx) error {
	sess, err := h.Store.Get(c)
	if err != nil {
		return c.Status(500).SendString(err.Error())
	}

	if err := sess.Destroy(); err != nil {
		return c.Status(500).SendString(err.Error())
	}

	return c.Redirect("/")
}
