package handlers

import (
	"backend/db"
	"backend/middleware"
	"encoding/json"
	"log"
	"net/http"
)

// GET /my-rooms
func GetMyRooms(w http.ResponseWriter, r *http.Request) {
	userID, err := middleware.ValidateToken(r)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	rows, err := db.Conn.Query(`
		SELECT r.id, r.room_name, r.is_group
		FROM chat_rooms r
		JOIN room_members m ON r.id = m.room_id
		WHERE m.user_id = $1
	`, userID)
	if err != nil {
		http.Error(w, "DBエラー", http.StatusInternalServerError)
		log.Println("❌ ルーム取得失敗:", err)
		return
	}
	defer rows.Close()

	type RoomInfo struct {
		ID       int    `json:"id"`
		RoomName string `json:"room_name"`
		IsGroup  bool   `json:"is_group"`
	}

	var rooms []RoomInfo
	for rows.Next() {
		var room RoomInfo
		err := rows.Scan(&room.ID, &room.RoomName, &room.IsGroup)
		if err != nil {
			http.Error(w, "読み込み失敗", http.StatusInternalServerError)
			return
		}
		rooms = append(rooms, room)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(rooms)
}
