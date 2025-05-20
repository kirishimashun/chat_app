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

		if err := conn.ReadJSON(&msg); err != nil {
			log.Println("websocketの接続終了:", err)
			break
		}

		log.Printf("📨 受信: %d → %d / %s", msg.SenderID, msg.ReceiverID, msg.Content)

		// メッセージをDBに保存（receiver_idは保存しない）
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

		// 相手（receiver_id）が接続していれば中継
		clientsMu.Lock()
		receiverConn, ok := clients[msg.ReceiverID]
		clientsMu.Unlock()

		if ok {
			if err := receiverConn.WriteJSON(msg); err != nil {
				log.Println("⚠️ 送信エラー:", err)
			}
		}
	}
}
