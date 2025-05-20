package models

type Mention struct {
	MessageID       int `json:"message_id"`        // メンション元のメッセージID
	MentionTargetID int `json:"mention_target_id"` // メンションされたユーザーID
}
