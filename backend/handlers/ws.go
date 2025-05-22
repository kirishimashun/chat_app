package handlers

import (
	"backend/db"
	"backend/middleware"
	"backend/models"
	"log"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

// WebSocket接続管理
var clients = make(map[int]*websocket.Conn)
var clientsMu sync.Mutex

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

func HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	userID, err := middleware.ValidateToken(r)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("WebSocket upgrade error:", err)
		return
	}

	clientsMu.Lock()
	clients[userID] = conn
	clientsMu.Unlock()

	log.Printf("✅ WebSocket接続: userID=%d", userID)

	go handleIncomingMessages(userID, conn)
}

func handleIncomingMessages(userID int, conn *websocket.Conn) {
	defer func() {
		conn.Close()
		clientsMu.Lock()
		delete(clients, userID)
		clientsMu.Unlock()
		log.Printf("👋 WebSocket切断: userID=%d", userID)
	}()

	for {
		var msg models.Message

		// メッセージを受信
		if err := conn.ReadJSON(&msg); err != nil {
			log.Println("WebSocketの接続終了:", err)
			break
		}

		log.Printf("📨 受信: %d → %s", msg.SenderID, msg.Content)

		// メッセージをDBに保存
		query := `
			INSERT INTO messages (room_id, sender_id, content)
			VALUES ($1, $2, $3)
			RETURNING id, created_at
		`
		err := db.Conn.QueryRow(query, msg.RoomID, msg.SenderID, msg.Content).
			Scan(&msg.ID, &msg.Timestamp)
		if err != nil {
			log.Println("❌ メッセージ保存失敗:", err)
			continue
		}

		// メッセージ送信時に未読データを挿入
		err = models.InsertMessageReads(db.Conn, msg.ID, msg.RoomID)
		if err != nil {
			log.Println("❌ 未読メッセージの挿入失敗:", err)
		}

		// room_idに参加している全メンバーにメッセージを送信
		clientsMu.Lock()
		// `room_id`に基づいて全メンバーを取得するロジックを追加
		members, err := models.GetRoomMembers(db.Conn, msg.RoomID)
		clientsMu.Unlock()

		if err != nil {
			log.Println("❌ ルームメンバー取得失敗:", err)
			continue
		}

		for _, member := range members {
			receiverConn, ok := clients[member.ID]
			if ok {
				if err := receiverConn.WriteJSON(msg); err != nil {
					log.Println("⚠️ 送信エラー:", err)
				}
			}
		}
	}
}
