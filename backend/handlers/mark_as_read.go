package handlers

import (
	"backend/db"
	"log"
	"net/http"
	"strconv"
	"time"
)

func MarkMessageAsRead(w http.ResponseWriter, r *http.Request) {
	messageIDStr := r.URL.Query().Get("message_id")
	userIDStr := r.URL.Query().Get("user_id")

	log.Println("Received message_id:", messageIDStr, "user_id:", userIDStr)

	if messageIDStr == "" || userIDStr == "" {
		http.Error(w, "message_id and user_id are required", http.StatusBadRequest)
		return
	}

	messageID, err := strconv.Atoi(messageIDStr)
	if err != nil {
		http.Error(w, "Invalid message_id", http.StatusBadRequest)
		return
	}
	userID, err := strconv.Atoi(userIDStr)
	if err != nil {
		http.Error(w, "Invalid user_id", http.StatusBadRequest)
		return
	}

	// 既読時間
	readAt := time.Now()

	// 存在チェック
	var exists bool
	err = db.Conn.QueryRow(`
		SELECT EXISTS (
			SELECT 1 FROM message_reads WHERE message_id = $1 AND user_id = $2
		)
	`, messageID, userID).Scan(&exists)
	if err != nil {
		http.Error(w, "DB error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if exists {
		_, err = db.Conn.Exec(`
			UPDATE message_reads SET read_at = $1
			WHERE message_id = $2 AND user_id = $3
		`, readAt, messageID, userID)
		if err != nil {
			http.Error(w, "UPDATE error: "+err.Error(), http.StatusInternalServerError)
			return
		}
		log.Printf("✅ UPDATE read_at: message_id=%d user_id=%d", messageID, userID)
	} else {
		_, err = db.Conn.Exec(`
			INSERT INTO message_reads (message_id, user_id, read_at)
			VALUES ($1, $2, $3)
		`, messageID, userID, readAt)
		if err != nil {
			http.Error(w, "INSERT error: "+err.Error(), http.StatusInternalServerError)
			return
		}
		log.Printf("✅ INSERT read_at: message_id=%d user_id=%d", messageID, userID)
	}

	// 🔧 メッセージ送信者を取得して通知
	var senderID int
	err = db.Conn.QueryRow(`SELECT sender_id FROM messages WHERE id = $1`, messageID).Scan(&senderID)
	if err == nil {
		NotifyUser(senderID, map[string]interface{}{
			"type":       "read",
			"message_id": messageID,
			"read_at":    readAt.Format(time.RFC3339),
		})
		log.Printf("📡 WebSocket通知: sender_id=%d message_id=%d", senderID, messageID)
	} else {
		log.Printf("❌ 送信者取得失敗: %v", err)
	}

	w.WriteHeader(http.StatusOK)
}
