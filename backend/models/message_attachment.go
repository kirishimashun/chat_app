package models

import "time"

type MessageAttachment struct {
	ID        int       `json:"id"`         // 添付ファイルのID（PK）
	MessageID int       `json:"message_id"` // 紐付くメッセージのID
	FileName  string    `json:"file_name"`  // 添付されたファイル名
	CreatedAt time.Time `json:"created_at"` // 作成日時（アップロード日時）
}
