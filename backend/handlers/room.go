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
	"time"
)

// GET /room?user_id=相手ID に対応するハンドラ
func GetOrCreateRoom(w http.ResponseWriter, r *http.Request) {
	currentUserID, err := middleware.ValidateToken(r)
	if err != nil {
		http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
		return
	}

	otherIDStr := r.URL.Query().Get("user_id")
	if otherIDStr == "" {
		http.Error(w, `{"error": "user_id が必要です"}`, http.StatusBadRequest)
		return
	}

	otherUserID, err := strconv.Atoi(otherIDStr)
	if err != nil {
		http.Error(w, `{"error": "user_id は数値である必要があります"}`, http.StatusBadRequest)
		return
	}

	roomID, err := getOrCreateRoomID(currentUserID, otherUserID)
	if err != nil {
		log.Println("❌ getOrCreateRoomID 失敗:", err)
		http.Error(w, `{"error": "ルーム取得に失敗しました"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int{"room_id": roomID})
}

// getOrCreateRoomID は 1対1チャット用のルームを取得または作成する
func getOrCreateRoomID(user1ID, user2ID int) (int, error) {
	var roomID int

	tx, err := db.Conn.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	err = tx.QueryRow(`
	SELECT rm.room_id
	FROM room_members rm
	JOIN chat_rooms cr ON rm.room_id = cr.id
	WHERE cr.is_group = 0
	  AND rm.room_id IN (
	    SELECT room_id FROM room_members WHERE user_id = $1
	    INTERSECT
	    SELECT room_id FROM room_members WHERE user_id = $2
	  )
	GROUP BY rm.room_id
	HAVING COUNT(*) = 2
`, user1ID, user2ID).Scan(&roomID)

	if err == sql.ErrNoRows {
		err = tx.QueryRow(`
    INSERT INTO chat_rooms (room_name, is_group, created_at, updated_at)
    VALUES ('', 0, NOW(), NOW())
    RETURNING id
`).Scan(&roomID)

		if err != nil {
			return 0, err
		}

		for _, uid := range []int{user1ID, user2ID} {
			_, err := tx.Exec(`
				INSERT INTO room_members (room_id, user_id, joined_at)
				VALUES ($1, $2, NOW())
			`, roomID, uid)
			if err != nil {
				return 0, err
			}
		}
	} else if err != nil {
		return 0, err
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return roomID, nil
}

// POST /rooms グループルーム作成ハンドラ
func CreateGroupRoom(w http.ResponseWriter, r *http.Request) {
	currentUserID, err := middleware.ValidateToken(r)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req models.CreateRoomRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	found := false
	for _, uid := range req.UserIDs {
		if uid == currentUserID {
			found = true
			break
		}
	}
	if !found {
		req.UserIDs = append(req.UserIDs, currentUserID)
	}

	tx, err := db.Conn.Begin()
	if err != nil {
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	var roomID int
	err = tx.QueryRow(`
		INSERT INTO chat_rooms (room_name, is_group, created_at, updated_at)
		VALUES ($1, 1, $2, $2)
		RETURNING id
	`, req.Name, time.Now()).Scan(&roomID)
	if err != nil {
		http.Error(w, "ルーム作成に失敗", http.StatusInternalServerError)
		log.Println("❌ chat_rooms INSERT 失敗:", err)
		return
	}

	stmt, err := tx.Prepare(`
		INSERT INTO room_members (room_id, user_id, joined_at)
		VALUES ($1, $2, $3)
	`)
	if err != nil {
		http.Error(w, "準備に失敗", http.StatusInternalServerError)
		return
	}
	defer stmt.Close()

	for _, uid := range req.UserIDs {
		if _, err := stmt.Exec(roomID, uid, time.Now()); err != nil {
			http.Error(w, "メンバー登録に失敗", http.StatusInternalServerError)
			return
		}
	}

	if err := tx.Commit(); err != nil {
		http.Error(w, "コミット失敗", http.StatusInternalServerError)
		return
	}

	log.Printf("✅ グループルーム作成: id=%d, name=%s, users=%v", roomID, req.Name, req.UserIDs)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int{"room_id": roomID})
}

// GET /group_rooms グループチャットだけ取得
func GetGroupRooms(w http.ResponseWriter, r *http.Request) {
	currentUserID, err := middleware.ValidateToken(r)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error":"Unauthorized"}`, http.StatusUnauthorized)
		return
	}

	rows, err := db.Conn.Query(`
		SELECT cr.id, cr.room_name, cr.is_group
		FROM chat_rooms cr
		JOIN room_members rm ON cr.id = rm.room_id
		WHERE rm.user_id = $1 AND cr.is_group = 1
	`, currentUserID)

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error":"DB error"}`, http.StatusInternalServerError)
		log.Println("❌ グループルーム取得失敗:", err)
		return
	}
	defer rows.Close()

	rooms := make([]models.ChatRoom, 0)

	for rows.Next() {
		var room models.ChatRoom
		if err := rows.Scan(&room.ID, &room.RoomName, &room.IsGroup); err != nil {
			w.Header().Set("Content-Type", "application/json")
			http.Error(w, `{"error":"スキャン失敗"}`, http.StatusInternalServerError)
			return
		}
		rooms = append(rooms, room)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(rooms)
}

// GET /room/members?room_id=xx
func GetRoomMembers(w http.ResponseWriter, r *http.Request) {
	_, err := middleware.ValidateToken(r)
	if err != nil {
		http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
		return
	}

	roomIDStr := r.URL.Query().Get("room_id")
	if roomIDStr == "" {
		http.Error(w, `{"error": "room_id が必要です"}`, http.StatusBadRequest)
		return
	}
	roomID, err := strconv.Atoi(roomIDStr)
	if err != nil {
		http.Error(w, `{"error": "room_id は数値である必要があります"}`, http.StatusBadRequest)
		return
	}

	rows, err := db.Conn.Query(`
		SELECT u.id, u.username
		FROM room_members rm
		JOIN users u ON rm.user_id = u.id
		WHERE rm.room_id = $1
	`, roomID)
	if err != nil {
		http.Error(w, `{"error": "DB error"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var members []models.User
	for rows.Next() {
		var u models.User
		if err := rows.Scan(&u.ID, &u.Username); err != nil {
			http.Error(w, `{"error": "Scan error"}`, http.StatusInternalServerError)
			return
		}
		members = append(members, u)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(members)
}

// GetUnreadCount は各ルームの未読数を返す
func GetUnreadCount(w http.ResponseWriter, r *http.Request) {
	userID, err := middleware.ValidateToken(r)
	if err != nil {
		http.Error(w, `{"error":"Unauthorized"}`, http.StatusUnauthorized)
		return
	}

	rows, err := db.Conn.Query(`
		SELECT m.room_id, COUNT(*) AS unread_count
		FROM messages m
		JOIN message_reads mr ON m.id = mr.message_id
		WHERE mr.user_id = $1 AND mr.read_at IS NULL
		GROUP BY m.room_id
	`, userID)
	if err != nil {
		http.Error(w, `{"error":"DB error"}`, http.StatusInternalServerError)
		log.Println("❌ 未読数取得失敗:", err)
		return
	}
	defer rows.Close()

	result := make(map[int]int)
	for rows.Next() {
		var roomID, count int
		if err := rows.Scan(&roomID, &count); err == nil {
			result[roomID] = count
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}
