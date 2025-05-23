package models

import (
	"database/sql"
	"fmt"
	"time"
)

// 既読・未読管理のための構造体
type MessageRead struct {
	MessageID int       `json:"message_id"`
	UserID    int       `json:"user_id"`
	Reaction  string    `json:"reaction"`
	ReadAt    time.Time `json:"read_at"`
}

type ReadUpdate struct {
	ID     int
	ReadAt time.Time
}

// メッセージ送信時に全メンバーに未読データを追加
func InsertMessageReads(db *sql.DB, messageID int, roomID int) error {
	members, err := GetRoomMembers(db, roomID)
	if err != nil {
		return fmt.Errorf("error retrieving room members: %v", err)
	}

	for _, member := range members {
		_, err := db.Exec(
			"INSERT INTO message_reads (message_id, user_id, read_at) VALUES ($1, $2, NULL)",
			messageID, member.ID,
		)
		if err != nil {
			return fmt.Errorf("error inserting unread message: %v", err)
		}
	}
	return nil
}

// 単一メッセージの既読処理
func MarkMessageAsRead(db *sql.DB, messageID int, userID int) error {
	_, err := db.Exec(
		"UPDATE message_reads SET read_at = $1 WHERE message_id = $2 AND user_id = $3",
		time.Now(), messageID, userID,
	)
	return err
}

// 全メッセージを既読にし、更新されたmessage_idとread_atを返す
func MarkAllMessagesAsRead(db *sql.DB, roomID int, userID int) ([]ReadUpdate, error) {
	rows, err := db.Query(`
		UPDATE message_reads
		SET read_at = NOW()
		WHERE message_id IN (
			SELECT m.id FROM messages m
			WHERE m.room_id = $1 AND m.sender_id != $2
		)
		AND user_id = $2 AND read_at IS NULL
		RETURNING message_id, read_at
	`, roomID, userID)
	if err != nil {
		return nil, fmt.Errorf("error marking messages as read: %v", err)
	}
	defer rows.Close()

	var updates []ReadUpdate
	for rows.Next() {
		var r ReadUpdate
		if err := rows.Scan(&r.ID, &r.ReadAt); err != nil {
			return nil, err
		}
		updates = append(updates, r)
	}
	return updates, nil
}

// ルームメンバーを取得
func GetRoomMembers(db *sql.DB, roomID int) ([]User, error) {
	var members []User
	query := "SELECT user_id FROM room_members WHERE room_id = $1"
	rows, err := db.Query(query, roomID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var member User
		if err := rows.Scan(&member.ID); err != nil {
			return nil, err
		}
		members = append(members, member)
	}
	return members, nil
}

// メッセージIDから送信者IDを取得
func GetSenderIDByMessageID(db *sql.DB, messageID int) (int, error) {
	var senderID int
	err := db.QueryRow("SELECT sender_id FROM messages WHERE id = $1", messageID).Scan(&senderID)
	if err != nil {
		return 0, fmt.Errorf("failed to get sender_id: %v", err)
	}
	return senderID, nil
}
