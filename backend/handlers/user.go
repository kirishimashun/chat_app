package handlers

import (
	"backend/db"
	"backend/middleware"
	"backend/models"

	"encoding/json"
	"net/http"
)

// 他ユーザー一覧を取得
func GetUsers(w http.ResponseWriter, r *http.Request) {
	userID, err := middleware.ValidateToken(r)
	if err != nil {
		http.Error(w, "Unauthorized: "+err.Error(), http.StatusUnauthorized)
		return
	}

	rows, err := db.Conn.Query("SELECT id, username FROM users WHERE id != $1", userID)
	if err != nil {
		http.Error(w, "ユーザー一覧の取得に失敗しました", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var users []models.User
	for rows.Next() {
		var u models.User
		if err := rows.Scan(&u.ID, &u.Username); err != nil {
			http.Error(w, "ユーザー情報の読み込みに失敗しました", http.StatusInternalServerError)
			return
		}
		users = append(users, u)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(users)
}
