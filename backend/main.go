package main

import (
	"log"
	"net/http"

	"github.com/gorilla/mux" // gorilla/muxパッケージをインポート
	"github.com/rs/cors"     // CORS設定を管理するrs/corsパッケージをインポート

	"backend/db"       // データベースを管理するパッケージ
	"backend/handlers" // HTTPリクエストのハンドラー関数を定義するパッケージ
)

func main() {
	db.Initialize()
	r := mux.NewRouter()

	// 🔐 認証
	r.HandleFunc("/signup", handlers.SignUp).Methods("POST")
	r.HandleFunc("/login", handlers.Login).Methods("POST")
	r.HandleFunc("/logout", handlers.Logout).Methods("POST")
	r.HandleFunc("/me", handlers.GetMe).Methods("GET")

	// 👤 ユーザー一覧
	r.HandleFunc("/users", handlers.GetUsers).Methods("GET")

	// 💬 メッセージ・ルーム関連
	r.HandleFunc("/messages", handlers.SendMessage).Methods("POST")
	r.HandleFunc("/messages", handlers.GetMessages).Methods("GET")
	r.HandleFunc("/room", handlers.GetOrCreateRoom).Methods("GET")             // 1対1チャット
	r.HandleFunc("/rooms", handlers.CreateGroupRoom).Methods("POST")           // グループチャット
	r.HandleFunc("/create-chat-room", handlers.CreateChatRoom).Methods("POST") // 旧名APIなら整理も検討
	r.HandleFunc("/my-rooms", handlers.GetMyRooms).Methods("GET")
	r.HandleFunc("/group_rooms", handlers.GetGroupRooms).Methods("GET")

	// 🌐 WebSocket
	r.HandleFunc("/ws", handlers.HandleWebSocket)

	// CORS設定
	handler := cors.New(cors.Options{
		AllowedOrigins:   []string{"http://localhost:3001"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Content-Type", "Authorization"},
		AllowCredentials: true,
	}).Handler(r)

	log.Println("✅ Server started at http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", handler))
}
