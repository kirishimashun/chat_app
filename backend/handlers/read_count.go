package handlers

import (
	"backend/db"
	"encoding/json"
	"fmt"
	"net/http"
)

// 既読人数を取得するハンドラー
func GetReadCount(w http.ResponseWriter, r *http.Request) {
	// room_idをクエリパラメータから取得
	roomID := r.URL.Query().Get("room_id")
	if roomID == "" {
		http.Error(w, "room_id is required", http.StatusBadRequest)
		return
	}

	// 既読人数を取得するSQLクエリ（message_readsテーブルとmessageテーブルを結合）
	query := `
		SELECT COUNT(DISTINCT mr.user_id) 
		FROM message_reads mr
		JOIN messages m ON mr.message_id = m.id
		WHERE m.room_id = $1 AND mr.read_at IS NOT NULL
	`

	var readCount int
	// データベースから既読ユーザー数を取得
	err := db.Conn.QueryRow(query, roomID).Scan(&readCount)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error fetching read count: %v", err), http.StatusInternalServerError)
		return
	}

	// 既読人数をJSON形式で返す
	response := map[string]int{"read_count": readCount}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
