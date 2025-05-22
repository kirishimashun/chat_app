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

// メッセージ送信時に全メンバーに未読データを追加
func InsertMessageReads(db *sql.DB, messageID int, roomID int) error {
	// ルームメンバーを取得
	members, err := GetRoomMembers(db, roomID)
	if err != nil {
		return fmt.Errorf("error retrieving room members: %v", err)
	}

	// 各メンバーに未読データを挿入
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

// ユーザーがメッセージを読んだ際にread_atを更新
func MarkMessageAsRead(db *sql.DB, messageID int, userID int) error {
	// 既読時間を設定
	_, err := db.Exec(
		"UPDATE message_reads SET read_at = $1 WHERE message_id = $2 AND user_id = $3",
		time.Now(), messageID, userID,
	)
	if err != nil {
		return fmt.Errorf("error marking message as read: %v", err)
	}

	return nil
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
