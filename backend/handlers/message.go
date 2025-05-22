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

	// 🔽 メッセージを保存
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

	// 🔽 ルームメンバーに未読データ挿入（自分自身含む）
	err = models.InsertMessageReads(db.Conn, msg.ID, msg.RoomID)
	if err != nil {
		log.Printf("⚠️ message_reads 挿入エラー: %v", err)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(msg)
}

// メッセージ取得（read_at付き）
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

	rows, err := db.Conn.Query(`
	    SELECT m.id, m.room_id, m.sender_id, m.content, m.created_at, mr.read_at
	    FROM messages m
	    LEFT JOIN message_reads mr
		    ON m.id = mr.message_id AND mr.user_id = $2
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
		ReadAt    *time.Time `json:"read_at,omitempty"`
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
