package handlers

import (
	"backend/db"
	"backend/middleware"
	"backend/models"
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

// POST /rooms グループルーム作成ハンドラ
func CreateGroupRoom(w http.ResponseWriter, r *http.Request) {
	// 認証
	currentUserID, err := middleware.ValidateToken(r)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// JSONデコード
	var req models.CreateRoomRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// 自分が含まれていないなら追加
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

	// トランザクションで挿入
	tx, err := db.Conn.Begin()
	if err != nil {
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	// ✅ SQL文をバッククォート `` で囲む（Goの文法）
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

	var rooms []models.ChatRoom
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
