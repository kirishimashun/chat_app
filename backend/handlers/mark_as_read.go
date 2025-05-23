package handlers

import (
	"backend/db"
	"log"
	"net/http"
	"strconv"
	"time"
)

// MarkMessageAsRead handles marking a specific message as read by a user.
func MarkMessageAsRead(w http.ResponseWriter, r *http.Request) {
	messageIDStr := r.URL.Query().Get("message_id")
	userIDStr := r.URL.Query().Get("user_id")

	log.Println("📥 MarkMessageAsRead: message_id=", messageIDStr, ", user_id=", userIDStr)

	if messageIDStr == "" || userIDStr == "" {
		http.Error(w, `{"error": "message_id and user_id are required"}`, http.StatusBadRequest)
		return
	}

	messageID, err := strconv.Atoi(messageIDStr)
	if err != nil {
		http.Error(w, `{"error": "Invalid message_id"}`, http.StatusBadRequest)
		return
	}
	userID, err := strconv.Atoi(userIDStr)
	if err != nil {
		http.Error(w, `{"error": "Invalid user_id"}`, http.StatusBadRequest)
		return
	}

	readAt := time.Now()

	// Try to update first
	res, err := db.Conn.Exec(`
		UPDATE message_reads SET read_at = $1
		WHERE message_id = $2 AND user_id = $3
	`, readAt, messageID, userID)
	if err != nil {
		http.Error(w, `{"error": "DB update error: `+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	numRows, _ := res.RowsAffected()
	if numRows == 0 {
		// If no rows updated, insert new entry
		_, err = db.Conn.Exec(`
			INSERT INTO message_reads (message_id, user_id, read_at)
			VALUES ($1, $2, $3)
		`, messageID, userID, readAt)
		if err != nil {
			http.Error(w, `{"error": "DB insert error: `+err.Error()+`"}`, http.StatusInternalServerError)
			return
		}
		log.Printf("✅ INSERT read_at: message_id=%d user_id=%d", messageID, userID)
	} else {
		log.Printf("✅ UPDATE read_at: message_id=%d user_id=%d", messageID, userID)
	}

	// Notify sender
	var senderID int
	err = db.Conn.QueryRow(`SELECT sender_id FROM messages WHERE id = $1`, messageID).Scan(&senderID)
	if err != nil {
		log.Printf("❌ 送信者取得失敗: %v", err)
	} else {
		NotifyUser(senderID, map[string]interface{}{
			"type":       "read",
			"message_id": messageID,
			"read_at":    readAt.Format(time.RFC3339),
		})
		log.Printf("📡 WebSocket通知: sender_id=%d message_id=%d", senderID, messageID)
	}

	w.WriteHeader(http.StatusOK)
}
