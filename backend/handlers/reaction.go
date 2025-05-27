package handlers

import (
	"backend/db"
	"backend/middleware"
	"encoding/json"
	"net/http"
)

type ReactionRequest struct {
	MessageID int    `json:"message_id"`
	Emoji     string `json:"emoji"`
}

func HandleReaction(w http.ResponseWriter, r *http.Request) {
	userID, err := middleware.ValidateToken(r)
	if err != nil {
		http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
		return
	}

	var req ReactionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error": "Invalid request"}`, http.StatusBadRequest)
		return
	}

	var current *string
	err = db.Conn.QueryRow(`
    SELECT reaction FROM message_reads
    WHERE message_id=$1 AND user_id=$2
  `, req.MessageID, userID).Scan(&current)

	if err != nil && err.Error() != "sql: no rows in result set" {
		http.Error(w, `{"error": "DB read failed"}`, http.StatusInternalServerError)
		return
	}

	if current != nil && *current == req.Emoji {
		// 取り消し
		_, err = db.Conn.Exec(`
      UPDATE message_reads SET reaction = NULL WHERE message_id=$1 AND user_id=$2
    `, req.MessageID, userID)
	} else if current == nil {
		// 追加
		_, err = db.Conn.Exec(`
      INSERT INTO message_reads (message_id, user_id, reaction, read_at)
      VALUES ($1, $2, $3, NOW())
    `, req.MessageID, userID, req.Emoji)
	} else {
		// 変更
		_, err = db.Conn.Exec(`
      UPDATE message_reads SET reaction = $3 WHERE message_id=$1 AND user_id=$2
    `, req.MessageID, userID, req.Emoji)
	}

	if err != nil {
		http.Error(w, `{"error": "DB update failed"}`, http.StatusInternalServerError)
		return
	}

	NotifyUser(userID, map[string]interface{}{
		"type":       "reaction",
		"message_id": req.MessageID,
		"emoji":      req.Emoji,
		"user_id":    userID,
	})

	w.WriteHeader(http.StatusOK)
}
