package handlers

import (
	"backend/db"
	"backend/middleware"
	"backend/models"
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type IncomingMessage struct {
	Content    string `json:"content"`
	ReceiverID int    `json:"receiver_id"`
}

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
	// メッセージ送信後にBroadcast
	// メッセージ送信後にBroadcast
	BroadcastMessage(msg.RoomID, msg.ID, msg.SenderID, msg.Content, msg.Timestamp)

	// 未読通知の追加
	rows, err := db.Conn.Query(`
	SELECT user_id FROM room_members
	WHERE room_id = $1 AND user_id != $2
`, msg.RoomID, msg.SenderID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var uid int
			if err := rows.Scan(&uid); err == nil {
				NotifyUser(uid, map[string]interface{}{
					"type":    "unread",
					"room_id": msg.RoomID,
				})
			}
		}
	}

	// メンション処理（@ユーザー名）
	mentionRegex := regexp.MustCompile(`@([\p{Hiragana}\p{Katakana}\p{Han}a-zA-Z0-9_]+)`)
	matches := mentionRegex.FindAllStringSubmatch(msg.Content, -1)
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		username := match[1]
		var mentionedUserID int
		err := db.Conn.QueryRow("SELECT id FROM users WHERE username = $1", username).Scan(&mentionedUserID)
		if err == nil && mentionedUserID != msg.SenderID {
			NotifyUser(mentionedUserID, map[string]interface{}{
				"type":    "mention",
				"user_id": msg.SenderID,
				"message": msg.Content,
			})
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(msg)
}

func EditMessage(w http.ResponseWriter, r *http.Request) {
	userID, err := middleware.ValidateToken(r)
	if err != nil {
		http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
		return
	}

	idStr := r.URL.Query().Get("id")
	messageID, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, `{"error": "Invalid message ID"}`, http.StatusBadRequest)
		return
	}

	var payload struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, `{"error": "Invalid request"}`, http.StatusBadRequest)
		return
	}

	_, err = db.Conn.Exec(`
        UPDATE messages
        SET content = $1, updated_at = NOW()
        WHERE id = $2 AND sender_id = $3
    `, payload.Content, messageID, userID)
	if err != nil {
		log.Println("❌ Edit failed:", err)
		http.Error(w, `{"error": "Failed to edit"}`, http.StatusInternalServerError)
		return
	}
	var roomID int
	err = db.Conn.QueryRow(`SELECT room_id FROM messages WHERE id = $1`, messageID).Scan(&roomID)
	if err == nil {
		BroadcastEdit(roomID, messageID, payload.Content)
	}
	w.WriteHeader(http.StatusOK)

	NotifyUser(userID, map[string]interface{}{
		"type":       "edit",
		"message_id": messageID,
		"content":    payload.Content,
	})

}

func DeleteMessage(w http.ResponseWriter, r *http.Request) {
	userID, err := middleware.ValidateToken(r)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	idStr := r.URL.Query().Get("id")
	if idStr == "" {
		http.Error(w, "id is required", http.StatusBadRequest)
		return
	}

	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	// 投稿者のユーザーIDと一致するか確認（セキュリティ）
	var senderID int
	err = db.Conn.QueryRow("SELECT sender_id FROM messages WHERE id = $1", id).Scan(&senderID)
	if err != nil {
		http.Error(w, "message not found", http.StatusNotFound)
		return
	}
	if senderID != userID {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	// メッセージ内容を論理削除
	_, err = db.Conn.Exec(`UPDATE messages SET content = 'このメッセージは削除されました' WHERE id = $1`, id)
	if err != nil {
		http.Error(w, "delete failed", http.StatusInternalServerError)
		return
	}

	var roomID int
	err = db.Conn.QueryRow(`SELECT room_id FROM messages WHERE id = $1`, id).Scan(&roomID)
	if err == nil {
		BroadcastDelete(roomID, id)
	}
	w.WriteHeader(http.StatusOK)

	NotifyUser(userID, map[string]interface{}{
		"type":       "delete",
		"message_id": id,
	})

}

// メッセージ取得（read_at + reactions付き）
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

	// 永続既読更新
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

	// 既読通知
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
	}

	// メッセージ取得
	type MessageWithStatus struct {
		ID        int        `json:"id"`
		RoomID    int        `json:"room_id"`
		SenderID  int        `json:"sender_id"`
		Content   string     `json:"content"`
		Timestamp time.Time  `json:"timestamp"`
		ReadAt    *time.Time `json:"read_at"`
		Reactions []struct {
			UserID int    `json:"user_id"`
			Emoji  string `json:"emoji"`
		} `json:"reactions"`
	}

	rows, err := db.Conn.Query(`
		SELECT id, room_id, sender_id, content, created_at
		FROM messages
		WHERE room_id = $1
		ORDER BY created_at ASC
	`, roomID)
	if err != nil {
		log.Println("❌ メッセージSELECT失敗:", err)
		http.Error(w, `{"error": "メッセージ取得に失敗しました"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	messages := []MessageWithStatus{}
	messageIDMap := make(map[int]*MessageWithStatus)
	for rows.Next() {
		var msg MessageWithStatus
		if err := rows.Scan(&msg.ID, &msg.RoomID, &msg.SenderID, &msg.Content, &msg.Timestamp); err != nil {
			log.Println("❌ rows.Scan失敗:", err)
			http.Error(w, `{"error": "読み込みエラー"}`, http.StatusInternalServerError)
			return
		}
		messages = append(messages, msg)
		messageIDMap[msg.ID] = &messages[len(messages)-1]
	}

	r2, err := db.Conn.Query(`
		SELECT message_id, user_id, reaction, read_at
		FROM message_reads
		WHERE message_id IN (
			SELECT id FROM messages WHERE room_id = $1
		)
	`, roomID)
	if err == nil {
		defer r2.Close()
		for r2.Next() {
			var mid, uid int
			var emoji sql.NullString
			var readAt sql.NullTime
			if err := r2.Scan(&mid, &uid, &emoji, &readAt); err == nil {
				if m, ok := messageIDMap[mid]; ok {
					if emoji.Valid {
						m.Reactions = append(m.Reactions, struct {
							UserID int    `json:"user_id"`
							Emoji  string `json:"emoji"`
						}{UserID: uid, Emoji: emoji.String})
					}
					if uid != userID && readAt.Valid {
						if m.ReadAt == nil || readAt.Time.Before(*m.ReadAt) {
							m.ReadAt = &readAt.Time
						}
					}

				}
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(messages)
}

// MarkAllAsRead は部屋単位のメッセージをすべて既読にする
func MarkAllAsRead(w http.ResponseWriter, r *http.Request) {
	userID, err := middleware.ValidateToken(r)
	if err != nil {
		http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
		return
	}

	var payload struct {
		RoomID    *int `json:"room_id"`
		MessageID *int `json:"message_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, `{"error": "Bad request"}`, http.StatusBadRequest)
		return
	}

	// === ① ルーム全体の既読処理 ===
	if payload.RoomID != nil {
		_, err = db.Conn.Exec(`
			UPDATE message_reads
			SET read_at = NOW()
			WHERE message_id IN (
				SELECT id FROM messages
				WHERE room_id = $1 AND sender_id != $2
			)
			AND user_id = $2 AND read_at IS NULL
		`, *payload.RoomID, userID)
		if err != nil {
			log.Println("❌ 既読UPDATE失敗:", err)
		}

		rows, err := db.Conn.Query(`
			SELECT m.id, m.sender_id, mr.read_at
			FROM messages m
			JOIN message_reads mr ON m.id = mr.message_id
			WHERE m.room_id = $1 AND mr.user_id = $2
				AND mr.read_at IS NOT NULL
				AND m.sender_id != $2
				AND mr.read_at > NOW() - INTERVAL '10 seconds'
		`, *payload.RoomID, userID)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var messageID, senderID int
				var readAt time.Time
				if err := rows.Scan(&messageID, &senderID, &readAt); err == nil {
					NotifyUser(senderID, map[string]interface{}{
						"type":       "read",
						"message_id": messageID,
						"read_at":    readAt.Format(time.RFC3339),
					})
					log.Printf("📡 既読通知: message_id=%d → sender_id=%d", messageID, senderID)
				}
			}
		}

		// === ② 単一メッセージの既読処理 ===
	} else if payload.MessageID != nil {
		_, err = db.Conn.Exec(`
			UPDATE message_reads
			SET read_at = NOW()
			WHERE message_id = $1 AND user_id = $2 AND read_at IS NULL
		`, *payload.MessageID, userID)
		if err != nil {
			log.Println("❌ 単一既読UPDATE失敗:", err)
		}

		var senderID int
		var readAt time.Time
		err = db.Conn.QueryRow(`
			SELECT m.sender_id, mr.read_at
			FROM messages m
			JOIN message_reads mr ON m.id = mr.message_id
			WHERE m.id = $1 AND mr.user_id = $2
		`, *payload.MessageID, userID).Scan(&senderID, &readAt)
		if err == nil {
			NotifyUser(senderID, map[string]interface{}{
				"type":       "read",
				"message_id": *payload.MessageID,
				"read_at":    readAt.Format(time.RFC3339),
			})
			log.Printf("📡 単一既読通知: message_id=%d → sender_id=%d", *payload.MessageID, senderID)
		}
	}

	w.WriteHeader(http.StatusOK)
}

// AddReaction はメッセージにリアクションを追加・更新・削除する
func AddReaction(w http.ResponseWriter, r *http.Request) {
	userID, err := middleware.ValidateToken(r)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var payload struct {
		MessageID int    `json:"message_id"`
		Emoji     string `json:"emoji"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "Invalid body", http.StatusBadRequest)
		return
	}

	var current *string
	err = db.Conn.QueryRow(`
		SELECT reaction FROM message_reads
		WHERE message_id = $1 AND user_id = $2
	`, payload.MessageID, userID).Scan(&current)

	if err != nil && err.Error() != "sql: no rows in result set" {
		http.Error(w, `{"error": "DB read failed"}`, http.StatusInternalServerError)
		return
	}

	if current != nil && *current == payload.Emoji {
		// 同じ絵文字ならリアクション削除
		_, err = db.Conn.Exec(`
			UPDATE message_reads SET reaction = NULL WHERE message_id=$1 AND user_id=$2
		`, payload.MessageID, userID)
	} else if current == nil {
		// 新規リアクション
		_, err = db.Conn.Exec(`
			INSERT INTO message_reads (message_id, user_id, reaction, read_at)
			VALUES ($1, $2, $3, NOW())
		`, payload.MessageID, userID, payload.Emoji)
	} else {
		// リアクション更新
		_, err = db.Conn.Exec(`
			UPDATE message_reads SET reaction = $3 WHERE message_id=$1 AND user_id=$2
		`, payload.MessageID, userID, payload.Emoji)
	}

	if err != nil {
		http.Error(w, `{"error": "DB update failed"}`, http.StatusInternalServerError)
		return
	}

	// WebSocket通知
	NotifyUser(userID, map[string]interface{}{
		"type":       "reaction",
		"message_id": payload.MessageID,
		"emoji":      payload.Emoji,
		"user_id":    userID,
	})

	w.WriteHeader(http.StatusOK)
}

// 完全削除: メッセージをDBから削除する
func HardDeleteMessage(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Query().Get("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	_, err = db.Conn.Exec("DELETE FROM messages WHERE id = $1", id)
	if err != nil {
		http.Error(w, "failed to delete message", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}
