package models

type ChatRoom struct {
	ID       int    `json:"id"`
	RoomName string `json:"room_name"` // ✅ DB・JSONともに "room_name"
	IsGroup  bool   `json:"is_group"`
}

type CreateRoomRequest struct {
	Name    string `json:"name"`     // グループ名
	UserIDs []int  `json:"user_ids"` // 招待するユーザーIDの配列
}
