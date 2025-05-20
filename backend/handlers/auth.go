package handlers

import (
	"backend/db"
	"backend/middleware"
	"backend/models"
	"encoding/json"
	"net/http"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

// ユーザー登録（サインアップ）
func SignUp(w http.ResponseWriter, r *http.Request) {
	var user models.User
	decoder := json.NewDecoder(r.Body)
	defer r.Body.Close()

	if err := decoder.Decode(&user); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// ユーザー名の重複チェック
	var existingID int
	query := `SELECT id FROM users WHERE username=$1`
	err := db.Conn.QueryRow(query, user.Username).Scan(&existingID)
	if err != nil && err.Error() != "sql: no rows in result set" {
		http.Error(w, "データベースエラー", http.StatusInternalServerError)
		return
	}
	if existingID != 0 {
		http.Error(w, "ユーザー名はすでに存在します", http.StatusConflict)
		return
	}

	// パスワードハッシュ化
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(user.PasswordHash), bcrypt.DefaultCost)
	if err != nil {
		http.Error(w, "パスワードハッシュ化に失敗しました", http.StatusInternalServerError)
		return
	}

	// 登録
	query = `INSERT INTO users (username, password_hash) VALUES ($1, $2) RETURNING id`
	err = db.Conn.QueryRow(query, user.Username, hashedPassword).Scan(&user.ID)
	if err != nil {
		http.Error(w, "ユーザー作成に失敗しました", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(user)
}

// ログイン（トークン生成）
// Login（Cookieベース版）
func Login(w http.ResponseWriter, r *http.Request) {
	var user models.User
	decoder := json.NewDecoder(r.Body)
	defer r.Body.Close()

	if err := decoder.Decode(&user); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var storedPasswordHash string
	var userID int
	query := `SELECT id, password_hash FROM users WHERE username=$1`
	err := db.Conn.QueryRow(query, user.Username).Scan(&userID, &storedPasswordHash)
	if err != nil {
		http.Error(w, "ユーザーが存在しないか、DBエラー", http.StatusUnauthorized)
		return
	}

	// パスワード検証
	err = bcrypt.CompareHashAndPassword([]byte(storedPasswordHash), []byte(user.PasswordHash))
	if err != nil {
		http.Error(w, "パスワードが間違っています", http.StatusUnauthorized)
		return
	}

	// JWT生成
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id": userID,
		"exp":     jwt.NewNumericDate(time.Now().Add(24 * time.Hour)), // 24時間有効
	})
	tokenString, err := token.SignedString(middleware.SecretKey)
	if err != nil {
		http.Error(w, "トークン生成に失敗しました", http.StatusInternalServerError)
		return
	}

	// ✅ Cookieにセット
	http.SetCookie(w, &http.Cookie{
		Name:     "token",
		Value:    tokenString,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   86400, // 24時間
	})

	w.WriteHeader(http.StatusOK)
}

func Logout(w http.ResponseWriter, r *http.Request) {
	// Cookie 無効化
	http.SetCookie(w, &http.Cookie{
		Name:     "token",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})

	// 応答
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"message": "ログアウトしました",
	})
}

// ログイン中のユーザー情報を返す
func GetMe(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("token")
	if err != nil {
		http.Error(w, "トークンがありません", http.StatusUnauthorized)
		return
	}

	token, err := jwt.Parse(cookie.Value, func(token *jwt.Token) (interface{}, error) {
		return middleware.SecretKey, nil
	})
	if err != nil || !token.Valid {
		http.Error(w, "無効なトークンです", http.StatusUnauthorized)
		return
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		http.Error(w, "クレームが取得できません", http.StatusUnauthorized)
		return
	}

	userID := int(claims["user_id"].(float64))

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int{"user_id": userID})
}
