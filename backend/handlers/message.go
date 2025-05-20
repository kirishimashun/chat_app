package handlers

import (
	"backend/db"
	"backend/middleware"
	"backend/models"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
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

	// ✅ JSONを受け取る
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

	// ✅ 正しい2人のuser_idからroom_idを取得
	roomID, err := getOrCreateRoomID(userID, req.ReceiverID)
	if err != nil {
		http.Error(w, `{"error": "ルーム取得失敗"}`, http.StatusInternalServerError)
		return
	}
	log.Printf("✅ RoomID=%d を取得", roomID)

	log.Printf("✉️ メッセージ送信: sender=%d, room_id=%d, content=%s", userID, roomID, req.Content)

	// ✅ メッセージ保存
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

	log.Println("✅ メッセージ保存成功:", msg.ID)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(msg)
}

// メッセージ取得
func GetMessages(w http.ResponseWriter, r *http.Request) {
	_, err := middleware.ValidateToken(r)
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
		SELECT id, sender_id, content, created_at
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

	messages := make([]models.Message, 0) // ← ここ！nilではなく空スライスで初期化

	for rows.Next() {
		var msg models.Message
		if err := rows.Scan(&msg.ID, &msg.SenderID, &msg.Content, &msg.Timestamp); err != nil {
			log.Println("❌ rows.Scan失敗:", err)
			http.Error(w, `{"error": "メッセージ読み込みエラー"}`, http.StatusInternalServerError)
			return
		}
		msg.RoomID = roomID
		messages = append(messages, msg)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(messages)
}

// ルームがなければ作る（2人のユーザー間）
func getOrCreateRoomID(user1ID, user2ID int) (int, error) {
	var roomID int

	log.Printf("🔍 getOrCreateRoomID: user1ID=%d, user2ID=%d", user1ID, user2ID)

	// トランザクション開始
	tx, err := db.Conn.Begin()
	if err != nil {
		log.Println("❌ トランザクション開始失敗:", err)
		return 0, err
	}

	// 既存のルームを検索
	err = tx.QueryRow(`
		SELECT room_id
		FROM room_members
		WHERE user_id IN ($1, $2)
		GROUP BY room_id
		HAVING COUNT(DISTINCT user_id) = 2
	`, user1ID, user2ID).Scan(&roomID)

	if err == sql.ErrNoRows {
		log.Println("ℹ️ ルームが存在しないため新規作成します")

		// チャットルーム作成
		log.Println("🛠️ chat_rooms に INSERT")
		err = tx.QueryRow(`
			INSERT INTO chat_rooms (room_name, is_group)
			VALUES ('', 0)
			RETURNING id
		`).Scan(&roomID)

		if err != nil {
			tx.Rollback()
			log.Println("❌ chat_rooms 作成失敗:", err)
			return 0, err
		}

		// 重複チェック付きでメンバー登録（UNIQUE制約なし対応）
		log.Println("🛠️ room_members に INSERT（重複チェックあり）")
		log.Printf("🔍 room_members に登録予定: room_id=%d, user1ID=%d, user2ID=%d", roomID, user1ID, user2ID)

		for _, uid := range []int{user1ID, user2ID} {
			// users テーブルに存在するか確認
			var userExists bool
			err := tx.QueryRow(`SELECT EXISTS (SELECT 1 FROM users WHERE id = $1)`, uid).Scan(&userExists)
			if err != nil {
				tx.Rollback()
				log.Printf("❌ users 存在確認失敗: user_id=%d, err=%v", uid, err)
				return 0, err
			}
			if !userExists {
				tx.Rollback()
				log.Printf("❌ user_id=%d は users テーブルに存在しません", uid)
				return 0, fmt.Errorf("user_id %d does not exist", uid)
			}

			// すでに登録済みかチェック
			var exists bool
			err = tx.QueryRow(`
				SELECT EXISTS (
					SELECT 1 FROM room_members WHERE room_id = $1 AND user_id = $2
				)
			`, roomID, uid).Scan(&exists)
			if err != nil {
				tx.Rollback()
				log.Println("❌ room_members チェック失敗:", err)
				return 0, err
			}

			if !exists {
				log.Printf("🧪 INSERT 実行: room_id=%d, user_id=%d", roomID, uid)
				_, err = tx.Exec(`
					INSERT INTO room_members (room_id, user_id) VALUES ($1, $2)
				`, roomID, uid)
				if err != nil {
					tx.Rollback()
					log.Println("❌ room_members 作成失敗:", err)
					return 0, err
				}
			}
		}

		log.Printf("✅ 新しい room_id=%d を作成", roomID)

	} else if err != nil {
		tx.Rollback()
		log.Println("❌ 既存ルーム取得に失敗:", err)
		return 0, err
	} else {
		log.Printf("✅ 既存の room_id=%d を使用", roomID)
	}

	// コミット
	err = tx.Commit()
	if err != nil {
		tx.Rollback()
		log.Println("❌ トランザクションコミット失敗:", err)
		return 0, err
	}

	return roomID, nil
}
