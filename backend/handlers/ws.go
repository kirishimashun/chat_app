package handlers

import (
	"backend/db"
	"backend/middleware"
	"backend/models"
	"encoding/json"
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
		var raw map[string]interface{}
		if err := conn.ReadJSON(&raw); err != nil {
			log.Println("WebSocketの接続終了:", err)
			break
		}

		msgType, ok := raw["type"].(string)
		if !ok {
			log.Println("⚠️ 無効なメッセージ形式（typeがない）:", raw)
			continue
		}

		switch msgType {
		case "message":
			var msg models.Message
			rawBytes, _ := json.Marshal(raw)
			if err := json.Unmarshal(rawBytes, &msg); err != nil {
				log.Println("❌ メッセージ変換失敗:", err)
				continue
			}

			log.Printf("📨 受信: %d → %s", msg.SenderID, msg.Content)

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

			err = models.InsertMessageReads(db.Conn, msg.ID, msg.RoomID)
			if err != nil {
				log.Println("❌ 未読挿入失敗:", err)
			}

			members, err := models.GetRoomMembers(db.Conn, msg.RoomID)
			if err != nil {
				log.Println("❌ ルームメンバー取得失敗:", err)
				continue
			}

			clientsMu.Lock()
			for _, member := range members {
				// ✅ 自分も含めて全員に送信
				if conn, ok := clients[member.ID]; ok {
					err := conn.WriteJSON(map[string]interface{}{
						"type":      "message",
						"id":        msg.ID,
						"room_id":   msg.RoomID,
						"sender_id": msg.SenderID,
						"content":   msg.Content,
						"timestamp": msg.Timestamp.Format("2006-01-02T15:04:05Z07:00"),
					})
					if err != nil {
						log.Println("⚠️ メッセージ送信エラー:", err)
					}
				}
			}
			clientsMu.Unlock()

		case "read":
			messageIDFloat, ok1 := raw["message_id"].(float64)
			readAtStr, ok2 := raw["read_at"].(string)
			if !ok1 || !ok2 {
				log.Println("⚠️ read メッセージ形式エラー:", raw)
				continue
			}
			messageID := int(messageIDFloat)
			log.Printf("📩 read 受信: userID=%d messageID=%d read_at=%s", userID, messageID, readAtStr)

			senderID, err := models.GetSenderIDByMessageID(db.Conn, messageID)
			if err != nil {
				log.Printf("❌ senderID取得失敗: messageID=%d err=%v", messageID, err)
				continue
			}
			if senderID == userID {
				continue
			}

			NotifyUser(senderID, map[string]interface{}{
				"type":       "read",
				"message_id": messageID,
				"read_at":    readAtStr,
			})

		default:
			log.Println("⚠️ 未対応のメッセージtype:", msgType)
		}
	}
}

// 特定ユーザーにWebSocketで通知
func NotifyUser(userID int, payload interface{}) {
	log.Printf("📡 NotifyUser呼び出し: userID=%d payload=%v", userID, payload)
	clientsMu.Lock()
	defer clientsMu.Unlock()

	if conn, ok := clients[userID]; ok {
		if err := conn.WriteJSON(payload); err != nil {
			log.Printf("⚠️ WebSocket通知エラー: userID=%d, err=%v", userID, err)
		} else {
			log.Printf("✅ WebSocket通知成功: userID=%d", userID)
		}
	} else {
		log.Printf("❌ WebSocket未接続: userID=%d", userID)
	}
}
