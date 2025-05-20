package utils

import (
	"backend/middleware"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// トークンを生成するユーティリティ関数
func GenerateJWT(userID int) (string, error) {
	claims := jwt.MapClaims{
		"user_id": userID,
		"exp":     time.Now().Add(24 * time.Hour).Unix(), // 有効期限: 24時間
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	tokenString, err := token.SignedString(middleware.SecretKey)
	if err != nil {
		return "", err
	}

	return tokenString, nil
}
