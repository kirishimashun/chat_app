// ✅ Go: handlers/ws.go - WebSocket で reaction を受信し処理する

package handlers

import (
	"backend/db"
	"backend/middleware"
	"backend/models"
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

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
				if conn, ok := clients[member.ID]; ok {
					// 📩 メッセージ通知
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

					// 📡 未読バッジ通知（自分以外）
					// 📡 未読バッジ通知（自分以外）
					if member.ID != msg.SenderID {
						var count int
						err := db.Conn.QueryRow(`
		SELECT COUNT(*) FROM message_reads mr
		JOIN messages m ON mr.message_id = m.id
		WHERE mr.user_id = $1 AND mr.read_at IS NULL AND m.room_id = $2
	`, member.ID, msg.RoomID).Scan(&count)
						if err != nil {
							log.Printf("❌ 未読数取得失敗: userID=%d roomID=%d err=%v", member.ID, msg.RoomID, err)
						} else {
							err := conn.WriteJSON(map[string]interface{}{
								"type":    "unread",
								"room_id": msg.RoomID,
								"count":   count,
							})
							if err != nil {
								log.Println("⚠️ 未読数送信エラー:", err)
							}
						}
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

			// ✅ 自分のバッジを即時更新（未読が減る）
			var roomID int
			err = db.Conn.QueryRow("SELECT room_id FROM messages WHERE id = $1", messageID).Scan(&roomID)
			if err != nil {
				log.Printf("❌ roomID取得失敗: messageID=%d err=%v", messageID, err)
			} else {
				NotifyUnreadCount(userID, roomID)
			}

		case "reaction":
			messageIDFloat, ok1 := raw["message_id"].(float64)
			emojiStr, ok2 := raw["emoji"].(string)
			if !ok1 || !ok2 {
				log.Println("⚠️ reaction メッセージ形式エラー:", raw)
				continue
			}
			messageID := int(messageIDFloat)
			log.Printf("📩 reaction 受信: userID=%d messageID=%d emoji=%s", userID, messageID, emojiStr)

			req := ReactionRequest{MessageID: messageID, Emoji: emojiStr}
			body, _ := json.Marshal(req)
			r := &http.Request{Body: io.NopCloser(bytes.NewReader(body))}
			HandleReaction(nil, r)

			senderID, err := models.GetSenderIDByMessageID(db.Conn, messageID)
			if err != nil {
				log.Printf("❌ senderID取得失敗: messageID=%d err=%v", messageID, err)
				continue
			}

			payload := map[string]interface{}{
				"type":       "reaction",
				"message_id": messageID,
				"emoji":      emojiStr,
			}

			// 🔁 自分にも通知（これが必要）
			NotifyUser(userID, payload)

			// 🔁 相手にも通知（同一人物でなければ）
			if senderID != userID {
				NotifyUser(senderID, payload)
			}

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

// メッセージ編集をルーム内全員に通知する
// handlers/ws.go

// BroadcastEdit は指定されたルームに編集通知を送信する
func BroadcastEdit(roomID int, messageID int, content string) {
	clientsMu.Lock()
	defer clientsMu.Unlock()

	for uid, conn := range clients {
		if conn != nil {
			err := conn.WriteJSON(map[string]interface{}{
				"type":       "edit",
				"message_id": messageID,
				"content":    content,
			})
			if err != nil {
				log.Printf("⚠️ BroadcastEdit失敗: userID=%d, err=%v", uid, err)
			}
		}
	}
}

// BroadcastDelete は指定されたルームに削除通知を送信する
func BroadcastDelete(roomID int, messageID int) {
	clientsMu.Lock()
	defer clientsMu.Unlock()

	for uid, conn := range clients {
		if conn != nil {
			err := conn.WriteJSON(map[string]interface{}{
				"type":       "delete",
				"message_id": messageID,
			})
			if err != nil {
				log.Printf("⚠️ BroadcastDelete失敗: userID=%d, err=%v", uid, err)
			}
		}
	}
}

func BroadcastMessage(roomID int, messageID int, senderID int, content string, createdAt time.Time) {
	msg := map[string]interface{}{
		"type":      "message",
		"id":        messageID,
		"room_id":   roomID,
		"sender_id": senderID,
		"content":   content,
		"read_at":   nil,
		"timestamp": createdAt.Format(time.RFC3339),
	}
	b, err := json.Marshal(msg)
	if err != nil {
		log.Printf("❌ BroadcastMessage JSONエンコード失敗: %v", err)
		return
	}

	// 全クライアントに送信
	clientsMu.Lock()
	defer clientsMu.Unlock()
	for uid, conn := range clients {
		if conn == nil {
			continue
		}
		if err := conn.WriteMessage(1, b); err != nil {
			log.Printf("⚠️ メッセージ送信失敗 userID=%d: %v", uid, err)
		} else {
			log.Printf("📩 BroadcastMessage: userID=%d に送信", uid)
		}
	}
}

func NotifyUnreadCount(userID int, roomID int) {
	var count int
	err := db.Conn.QueryRow(`
		SELECT COUNT(*) FROM message_reads mr
		JOIN messages m ON mr.message_id = m.id
		WHERE mr.user_id = $1 AND mr.read_at IS NULL AND m.room_id = $2
	`, userID, roomID).Scan(&count)
	if err != nil {
		log.Printf("❌ NotifyUnreadCount失敗: userID=%d roomID=%d err=%v", userID, roomID, err)
		return
	}

	payload := map[string]interface{}{
		"type":    "unread",
		"room_id": roomID,
		"count":   count,
	}
	NotifyUser(userID, payload)
}
