package models

import "time"

type Message struct {
	ID         int       `json:"id"`
	RoomID     int       `json:"room_id"`
	SenderID   int       `json:"sender_id"`
	ReceiverID int       `json:"receiver_id"`
	Content    string    `json:"content"`
	Timestamp  time.Time `json:"timestamp"`
}
