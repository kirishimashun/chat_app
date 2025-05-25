package handlers

import (
	"backend/db"
	"backend/middleware"
	"backend/models"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type IncomingMessage struct {
	Content    string `json:"content"`
	ReceiverID int    `json:"receiver_id"`
}

// メッセージ送信
func SendMessage(w http.ResponseWriter, r *http.Request) {
	userID, err := middleware.ValidateToken(r)
	if err != nil {
		http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
		return
	}

	var req IncomingMessage
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error": "Bad request"}`, http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	if strings.TrimSpace(req.Content) == "" {
		http.Error(w, `{"error": "メッセージが空です"}`, http.StatusBadRequest)
		return
	}

	roomID, err := getOrCreateRoomID(userID, req.ReceiverID)
	if err != nil {
		http.Error(w, `{"error": "ルーム取得失敗"}`, http.StatusInternalServerError)
		return
	}
	log.Printf("✅ RoomID=%d を取得", roomID)

	var msg models.Message
	msg.SenderID = userID
	msg.RoomID = roomID
	msg.Content = req.Content

	err = db.Conn.QueryRow(`
		INSERT INTO messages (sender_id, room_id, content, created_at)
		VALUES ($1, $2, $3, NOW())
		RETURNING id, created_at
	`, msg.SenderID, msg.RoomID, msg.Content).Scan(&msg.ID, &msg.Timestamp)
	if err != nil {
		log.Println("❌ メッセージ保存失敗:", err)
		http.Error(w, `{"error": "保存失敗"}`, http.StatusInternalServerError)
		return
	}
	log.Printf("✅ メッセージ保存成功: messageID=%d", msg.ID)

	err = models.InsertMessageReads(db.Conn, msg.ID, msg.RoomID)
	if err != nil {
		log.Printf("⚠️ message_reads 挿入エラー: %v", err)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(msg)
}

// メッセージ取得（read_at付き）+ 既読反映 + WebSocket通知
func GetMessages(w http.ResponseWriter, r *http.Request) {
	userID, err := middleware.ValidateToken(r)
	if err != nil {
		http.Error(w, `{"error": "Unauthorized: `+err.Error()+`"}`, http.StatusUnauthorized)
		return
	}

	roomIDStr := r.URL.Query().Get("room_id")
	if roomIDStr == "" || roomIDStr == "null" {
		http.Error(w, `{"error": "room_id は必須です"}`, http.StatusBadRequest)
		return
	}

	roomID, err := strconv.Atoi(roomIDStr)
	if err != nil {
		http.Error(w, `{"error": "room_id の形式が正しくありません"}`, http.StatusBadRequest)
		return
	}

	log.Printf("📥 メッセージ取得: roomID=%d", roomID)

	_, err = db.Conn.Exec(`
		UPDATE message_reads
		SET read_at = NOW()
		WHERE message_id IN (
			SELECT id FROM messages
			WHERE room_id = $1 AND sender_id != $2
		)
		AND user_id = $2 AND read_at IS NULL
	`, roomID, userID)
	if err != nil {
		log.Println("❌ 既読UPDATE失敗:", err)
	}

	rowsNotify, err := db.Conn.Query(`
		SELECT m.id, m.sender_id, mr.read_at
		FROM messages m
		JOIN message_reads mr ON m.id = mr.message_id
		WHERE m.room_id = $1 AND mr.user_id = $2
		  AND mr.read_at IS NOT NULL
		  AND m.sender_id != $2
		  AND mr.read_at > NOW() - INTERVAL '10 seconds'
	`, roomID, userID)
	if err == nil {
		defer rowsNotify.Close()
		for rowsNotify.Next() {
			var messageID, senderID int
			var readAt time.Time
			if err := rowsNotify.Scan(&messageID, &senderID, &readAt); err == nil {
				NotifyUser(senderID, map[string]interface{}{
					"type":       "read",
					"message_id": messageID,
					"read_at":    readAt.Format(time.RFC3339),
				})
				log.Printf("📡 既読通知: message_id=%d → sender_id=%d", messageID, senderID)
			}
		}
	} else {
		log.Println("❌ 既読通知SELECT失敗:", err)
	}

	rows, err := db.Conn.Query(`
	SELECT m.id, m.room_id, m.sender_id, m.content, m.created_at, mr.read_at
	FROM messages m
	LEFT JOIN message_reads mr
		ON m.id = mr.message_id AND mr.user_id != $2
	WHERE m.room_id = $1
	ORDER BY m.created_at ASC
`, roomID, userID)

	if err != nil {
		log.Println("❌ メッセージSELECT失敗:", err)
		http.Error(w, `{"error": "メッセージ取得に失敗しました"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type MessageWithRead struct {
		ID        int        `json:"id"`
		RoomID    int        `json:"room_id"`
		SenderID  int        `json:"sender_id"`
		Content   string     `json:"content"`
		Timestamp time.Time  `json:"timestamp"`
		ReadAt    *time.Time `json:"read_at"`
	}

	messages := make([]MessageWithRead, 0)
	for rows.Next() {
		var msg MessageWithRead
		if err := rows.Scan(&msg.ID, &msg.RoomID, &msg.SenderID, &msg.Content, &msg.Timestamp, &msg.ReadAt); err != nil {
			log.Println("❌ rows.Scan失敗:", err)
			http.Error(w, `{"error": "読み込みエラー"}`, http.StatusInternalServerError)
			return
		}
		messages = append(messages, msg)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(messages)
}

func MarkAllAsRead(w http.ResponseWriter, r *http.Request) {
	userID, err := middleware.ValidateToken(r)
	if err != nil {
		http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
		return
	}

	var payload struct {
		RoomID    int `json:"room_id"`
		MessageID int `json:"message_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, `{"error": "Invalid request body"}`, http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	if payload.MessageID != 0 {
		log.Printf("📥 単一メッセージ既読リクエスト: userID=%d messageID=%d", userID, payload.MessageID)
		_, err := db.Conn.Exec(`
			UPDATE message_reads
			SET read_at = NOW()
			WHERE message_id = $1 AND user_id = $2 AND read_at IS NULL
		`, payload.MessageID, userID)
		if err != nil {
			log.Printf("❌ 単一既読UPDATE失敗: %v", err)
			http.Error(w, `{"error": "DB update failed"}`, http.StatusInternalServerError)
			return
		}

		senderID, err := models.GetSenderIDByMessageID(db.Conn, payload.MessageID)
		if err == nil && senderID != userID {
			readAt := time.Now().Format(time.RFC3339)
			NotifyUser(senderID, map[string]interface{}{
				"type":       "read",
				"message_id": payload.MessageID,
				"read_at":    readAt,
			})
			log.Printf("📡 既読通知: message_id=%d → sender_id=%d", payload.MessageID, senderID)
		}

		w.WriteHeader(http.StatusOK)
		return
	}

	log.Printf("📥 既読リクエスト: userID=%d roomID=%d", userID, payload.RoomID)

	updated, err := models.MarkAllMessagesAsRead(db.Conn, payload.RoomID, userID)
	if err != nil {
		log.Printf("❌ MarkAllMessagesAsRead 失敗: %v", err)
		http.Error(w, `{"error": "DB update failed"}`, http.StatusInternalServerError)
		return
	}

	log.Printf("📦 updated: %+v", updated) // ←★ これを追加

	for _, record := range updated {
		senderID, err := models.GetSenderIDByMessageID(db.Conn, record.ID)
		if err != nil {
			log.Printf("❌ sender_id取得失敗: message_id=%d err=%v", record.ID, err)
			continue
		}
		if senderID == userID {
			continue
		}
		NotifyUser(senderID, map[string]interface{}{
			"type":       "read",
			"message_id": record.ID,
			"read_at":    record.ReadAt.Format(time.RFC3339),
		})
		log.Printf("📡 既読通知: message_id=%d → sender_id=%d", record.ID, senderID)
	}

	w.WriteHeader(http.StatusOK)
}
