package middleware

import (
	"errors"
	"net/http"

	"github.com/golang-jwt/jwt/v5"
)

var SecretKey = []byte("your-secret-key") // 本番環境では .env や設定から取得

// ✅ Cookieからトークンを検証して user_id を返す関数
func ValidateToken(r *http.Request) (int, error) {
	// ✅ Authorizationヘッダーではなく Cookie から取得
	cookie, err := r.Cookie("token")
	if err != nil {
		return 0, errors.New("Cookie 'token' が見つかりません")
	}

	// トークンをパース・検証
	token, err := jwt.Parse(cookie.Value, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("署名方法が無効です")
		}
		return SecretKey, nil
	})

	if err != nil || !token.Valid {
		return 0, errors.New("トークンが無効です")
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return 0, errors.New("クレームの解析に失敗しました")
	}

	userIDFloat, ok := claims["user_id"].(float64)
	if !ok {
		return 0, errors.New("user_id が見つかりません")
	}

	return int(userIDFloat), nil
}
