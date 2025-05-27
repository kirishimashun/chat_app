package handlers

import (
	"backend/db"
	"backend/middleware"
	"backend/models"
	"database/sql"
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
					if uid != userID && readAt.Valid && m.ReadAt == nil {
						m.ReadAt = &readAt.Time
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

// package handlers

// import (
// 	"backend/db"
// 	"backend/middleware"
// 	"backend/models"
// 	"encoding/json"
// 	"log"
// 	"net/http"
// 	"strconv"
// 	"strings"
// 	"time"
// )

// type IncomingMessage struct {
// 	Content    string `json:"content"`
// 	ReceiverID int    `json:"receiver_id"`
// }

// // メッセージ送信
// func SendMessage(w http.ResponseWriter, r *http.Request) {
// 	userID, err := middleware.ValidateToken(r)
// 	if err != nil {
// 		http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
// 		return
// 	}

// 	var req IncomingMessage
// 	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
// 		http.Error(w, `{"error": "Bad request"}`, http.StatusBadRequest)
// 		return
// 	}
// 	defer r.Body.Close()

// 	if strings.TrimSpace(req.Content) == "" {
// 		http.Error(w, `{"error": "メッセージが空です"}`, http.StatusBadRequest)
// 		return
// 	}

// 	roomID, err := getOrCreateRoomID(userID, req.ReceiverID)
// 	if err != nil {
// 		http.Error(w, `{"error": "ルーム取得失敗"}`, http.StatusInternalServerError)
// 		return
// 	}
// 	log.Printf("✅ RoomID=%d を取得", roomID)

// 	var msg models.Message
// 	msg.SenderID = userID
// 	msg.RoomID = roomID
// 	msg.Content = req.Content

// 	err = db.Conn.QueryRow(`
// 		INSERT INTO messages (sender_id, room_id, content, created_at)
// 		VALUES ($1, $2, $3, NOW())
// 		RETURNING id, created_at
// 	`, msg.SenderID, msg.RoomID, msg.Content).Scan(&msg.ID, &msg.Timestamp)
// 	if err != nil {
// 		log.Println("❌ メッセージ保存失敗:", err)
// 		http.Error(w, `{"error": "保存失敗"}`, http.StatusInternalServerError)
// 		return
// 	}
// 	log.Printf("✅ メッセージ保存成功: messageID=%d", msg.ID)

// 	err = models.InsertMessageReads(db.Conn, msg.ID, msg.RoomID)
// 	if err != nil {
// 		log.Printf("⚠️ message_reads 挿入エラー: %v", err)
// 	}

// 	w.Header().Set("Content-Type", "application/json")
// 	json.NewEncoder(w).Encode(msg)
// }

// // メッセージ取得（read_at付き）+ 既読反映 + WebSocket通知
// func GetMessages(w http.ResponseWriter, r *http.Request) {
// 	userID, err := middleware.ValidateToken(r)
// 	if err != nil {
// 		http.Error(w, `{"error": "Unauthorized: `+err.Error()+`"}`, http.StatusUnauthorized)
// 		return
// 	}

// 	roomIDStr := r.URL.Query().Get("room_id")
// 	if roomIDStr == "" || roomIDStr == "null" {
// 		http.Error(w, `{"error": "room_id は必須です"}`, http.StatusBadRequest)
// 		return
// 	}

// 	roomID, err := strconv.Atoi(roomIDStr)
// 	if err != nil {
// 		http.Error(w, `{"error": "room_id の形式が正しくありません"}`, http.StatusBadRequest)
// 		return
// 	}

// 	log.Printf("📥 メッセージ取得: roomID=%d", roomID)

// 	// 既読更新
// 	_, err = db.Conn.Exec(`
// 		UPDATE message_reads
// 		SET read_at = NOW()
// 		WHERE message_id IN (
// 			SELECT id FROM messages
// 			WHERE room_id = $1 AND sender_id != $2
// 		)
// 		AND user_id = $2 AND read_at IS NULL
// 	`, roomID, userID)
// 	if err != nil {
// 		log.Println("❌ 既読UPDATE失敗:", err)
// 	}

// 	// WebSocket既読通知
// 	rowsNotify, err := db.Conn.Query(`
// 		SELECT m.id, m.sender_id, mr.read_at
// 		FROM messages m
// 		JOIN message_reads mr ON m.id = mr.message_id
// 		WHERE m.room_id = $1 AND mr.user_id = $2
// 		  AND mr.read_at IS NOT NULL
// 		  AND m.sender_id != $2
// 		  AND mr.read_at > NOW() - INTERVAL '10 seconds'
// 	`, roomID, userID)
// 	if err == nil {
// 		defer rowsNotify.Close()
// 		for rowsNotify.Next() {
// 			var messageID, senderID int
// 			var readAt time.Time
// 			if err := rowsNotify.Scan(&messageID, &senderID, &readAt); err == nil {
// 				NotifyUser(senderID, map[string]interface{}{
// 					"type":       "read",
// 					"message_id": messageID,
// 					"read_at":    readAt.Format(time.RFC3339),
// 				})
// 				log.Printf("📡 既読通知: message_id=%d → sender_id=%d", messageID, senderID)
// 			}
// 		}
// 	}

// 	// メッセージ本体＋自分のreaction＋他人からのread_atを取得
// 	rows, err := db.Conn.Query(`
// 		SELECT m.id, m.room_id, m.sender_id, m.content, m.created_at,
// 			(SELECT read_at FROM message_reads WHERE message_id = m.id AND user_id != $2 AND read_at IS NOT NULL ORDER BY read_at DESC LIMIT 1) as other_read_at,
// 			(SELECT reaction FROM message_reads WHERE message_id = m.id AND user_id = $2 LIMIT 1) as my_reaction
// 		FROM messages m
// 		WHERE m.room_id = $1
// 		ORDER BY m.created_at ASC
// 	`, roomID, userID)

// 	if err != nil {
// 		log.Println("❌ メッセージSELECT失敗:", err)
// 		http.Error(w, `{"error": "メッセージ取得に失敗しました"}`, http.StatusInternalServerError)
// 		return
// 	}
// 	defer rows.Close()

// 	type MessageWithStatus struct {
// 		ID        int        `json:"id"`
// 		RoomID    int        `json:"room_id"`
// 		SenderID  int        `json:"sender_id"`
// 		Content   string     `json:"content"`
// 		Timestamp time.Time  `json:"timestamp"`
// 		ReadAt    *time.Time `json:"read_at"`  // 他人による既読
// 		Reaction  *string    `json:"reaction"` // 自分がつけたリアクション
// 	}

// 	messages := make([]MessageWithStatus, 0)
// 	for rows.Next() {
// 		var msg MessageWithStatus
// 		if err := rows.Scan(&msg.ID, &msg.RoomID, &msg.SenderID, &msg.Content, &msg.Timestamp, &msg.ReadAt, &msg.Reaction); err != nil {
// 			log.Println("❌ rows.Scan失敗:", err)
// 			http.Error(w, `{"error": "読み込みエラー"}`, http.StatusInternalServerError)
// 			return
// 		}
// 		messages = append(messages, msg)
// 	}

// 	w.Header().Set("Content-Type", "application/json")
// 	json.NewEncoder(w).Encode(messages)
// }

// func MarkAllAsRead(w http.ResponseWriter, r *http.Request) {
// 	userID, err := middleware.ValidateToken(r)
// 	if err != nil {
// 		http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
// 		return
// 	}

// 	var payload struct {
// 		RoomID    int `json:"room_id"`
// 		MessageID int `json:"message_id"`
// 	}
// 	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
// 		http.Error(w, `{"error": "Invalid request body"}`, http.StatusBadRequest)
// 		return
// 	}
// 	defer r.Body.Close()

// 	if payload.MessageID != 0 {
// 		log.Printf("📥 単一メッセージ既読リクエスト: userID=%d messageID=%d", userID, payload.MessageID)
// 		_, err := db.Conn.Exec(`
// 			UPDATE message_reads
// 			SET read_at = NOW()
// 			WHERE message_id = $1 AND user_id = $2 AND read_at IS NULL
// 		`, payload.MessageID, userID)
// 		if err != nil {
// 			log.Printf("❌ 単一既読UPDATE失敗: %v", err)
// 			http.Error(w, `{"error": "DB update failed"}`, http.StatusInternalServerError)
// 			return
// 		}

// 		senderID, err := models.GetSenderIDByMessageID(db.Conn, payload.MessageID)
// 		if err == nil && senderID != userID {
// 			readAt := time.Now().Format(time.RFC3339)
// 			NotifyUser(senderID, map[string]interface{}{
// 				"type":       "read",
// 				"message_id": payload.MessageID,
// 				"read_at":    readAt,
// 			})
// 			log.Printf("📡 既読通知: message_id=%d → sender_id=%d", payload.MessageID, senderID)
// 		}

// 		w.WriteHeader(http.StatusOK)
// 		return
// 	}

// 	log.Printf("📥 既読リクエスト: userID=%d roomID=%d", userID, payload.RoomID)

// 	updated, err := models.MarkAllMessagesAsRead(db.Conn, payload.RoomID, userID)
// 	if err != nil {
// 		log.Printf("❌ MarkAllMessagesAsRead 失敗: %v", err)
// 		http.Error(w, `{"error": "DB update failed"}`, http.StatusInternalServerError)
// 		return
// 	}

// 	log.Printf("📦 updated: %+v", updated) // ←★ これを追加

// 	for _, record := range updated {
// 		senderID, err := models.GetSenderIDByMessageID(db.Conn, record.ID)
// 		if err != nil {
// 			log.Printf("❌ sender_id取得失敗: message_id=%d err=%v", record.ID, err)
// 			continue
// 		}
// 		if senderID == userID {
// 			continue
// 		}
// 		NotifyUser(senderID, map[string]interface{}{
// 			"type":       "read",
// 			"message_id": record.ID,
// 			"read_at":    record.ReadAt.Format(time.RFC3339),
// 		})
// 		log.Printf("📡 既読通知: message_id=%d → sender_id=%d", record.ID, senderID)
// 	}

// 	w.WriteHeader(http.StatusOK)
// }

// func AddReaction(w http.ResponseWriter, r *http.Request) {
// 	userID, err := middleware.ValidateToken(r)
// 	if err != nil {
// 		http.Error(w, "Unauthorized", http.StatusUnauthorized)
// 		return
// 	}

// 	var payload struct {
// 		MessageID int    `json:"message_id"`
// 		Emoji     string `json:"emoji"`
// 	}
// 	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
// 		http.Error(w, "Invalid body", http.StatusBadRequest)
// 		return
// 	}

// 	// reaction情報を仮のメモリ or 永続管理層に格納する必要あり（今回は通知のみ）
// 	senderID, err := models.GetSenderIDByMessageID(db.Conn, payload.MessageID)
// 	if err == nil && senderID != userID {
// 		NotifyUser(senderID, map[string]interface{}{
// 			"type":       "reaction",
// 			"message_id": payload.MessageID,
// 			"emoji":      payload.Emoji,
// 		})
// 	}

// 	log.Printf("📡 リアクション通知: message_id=%d → sender_id=%d (%s)", payload.MessageID, senderID, payload.Emoji)

// 	w.WriteHeader(http.StatusOK)
// }
