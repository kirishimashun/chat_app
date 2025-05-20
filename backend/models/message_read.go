package models

import "time"

type MessageRead struct {
	MessageID int       `json:"message_id"` // 対象メッセージのID
	UserID    int       `json:"user_id"`    // 読んだユーザーのID
	Reaction  string    `json:"reaction"`   // リアクション（スタンプや絵文字など）
	ReadAt    time.Time `json:"read_at"`    // 既読日時
}
