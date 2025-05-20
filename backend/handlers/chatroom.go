package handlers

import (
	"backend/db"
	"backend/middleware"
	"backend/models"
	"encoding/json"
	"log"
	"net/http"
)

// チャットルーム作成（グループ対応）
func CreateChatRoom(w http.ResponseWriter, r *http.Request) {
	userID, err := middleware.ValidateToken(r)
	if err != nil {
		http.Error(w, `{"error": "Unauthorized: `+err.Error()+`"}`, http.StatusUnauthorized)
		return
	}
	log.Println("✅ sender userID =", userID)

	var room models.ChatRoom
	if err := json.NewDecoder(r.Body).Decode(&room); err != nil {
		http.Error(w, `{"error": "リクエストの解析に失敗しました"}`, http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	isGroupInt := 0
	if room.IsGroup {
		isGroupInt = 1
	}

	query := `INSERT INTO chat_rooms (room_name, is_group) VALUES ($1, $2) RETURNING id`
	err = db.Conn.QueryRow(query, room.RoomName, isGroupInt).Scan(&room.ID)
	if err != nil {
		http.Error(w, `{"error": "チャットルームの作成に失敗しました"}`, http.StatusInternalServerError)
		return
	}

	memberQuery := `INSERT INTO room_members (room_id, user_id) VALUES ($1, $2)`
	_, err = db.Conn.Exec(memberQuery, room.ID, userID)
	if err != nil {
		http.Error(w, `{"error": "ルームメンバー追加に失敗しました"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(room)
}
