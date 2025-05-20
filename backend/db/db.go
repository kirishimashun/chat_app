package db

import (
	"database/sql"
	"fmt"
	"log"
	"time"

	_ "github.com/lib/pq"
)

var Conn *sql.DB

func Initialize() {
	var err error

	host := "db"
	port := 5432
	user := "chatuser"
	password := "password"
	dbname := "chat_app_db"

	connStr := fmt.Sprintf(
		"user=%s dbname=%s password=%s host=%s port=%d sslmode=disable",
		user, dbname, password, host, port,
	)

	// 最大10回、接続をリトライ
	for i := 0; i < 10; i++ {
		Conn, err = sql.Open("postgres", connStr)
		if err != nil {
			log.Printf("❌ DB接続失敗 (%d回目): %v", i+1, err)
		} else if err = Conn.Ping(); err == nil {
			log.Println("✅ DB接続成功")
			return
		} else {
			log.Printf("🔁 DB ping失敗 (%d回目): %v", i+1, err)
		}
		time.Sleep(3 * time.Second)
	}

	log.Fatal("❌ DBへの接続に10回失敗しました。サービスを停止します。")
}
