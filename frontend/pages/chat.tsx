'use client';
import { useEffect, useState, useRef } from "react";
import { useRouter } from "next/navigation";
import styles from "./ChatPage.module.css";

type User = {
  id: number;
  username: string;
};

type Message = {
  sender_id: number;
  content: string;
};

type RoomInfo = {
  id: number;
  room_name: string;
  is_group: boolean;
};

export default function ChatPage() {
  const [users, setUsers] = useState<User[]>([]);
  const [selectedUser, setSelectedUser] = useState<User | null>(null);
  const [messages, setMessages] = useState<Message[]>([]);
  const [messageText, setMessageText] = useState("");
  const [userId, setUserId] = useState<number | null>(null);
  const [roomId, setRoomId] = useState<number | null>(null);
  const [socket, setSocket] = useState<WebSocket | null>(null);
  const [groupRooms, setGroupRooms] = useState<RoomInfo[]>([]);
  const router = useRouter();
  const messageEndRef = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    fetch("http://localhost:8080/me", { credentials: "include" })
      .then(res => res.json())
      .then(data => setUserId(Number(data.user_id)))
      .catch(() => router.push("/login"));
  }, []);

  useEffect(() => {
    if (!userId) return;
    const ws = new WebSocket("ws://localhost:8080/ws");
    ws.onmessage = (event) => {
      const data = JSON.parse(event.data);
      setMessages(prev => [...prev, data]);
    };
    setSocket(ws);
    return () => ws.close();
  }, [userId]);

  useEffect(() => {
    if (!userId) return;

    // ユーザー一覧取得
    fetch("http://localhost:8080/users", { credentials: "include" })
      .then(res => res.json())
      .then(setUsers)
      .catch(err => {
        console.error("ユーザー取得失敗:", err);
        setUsers([]);
      });

    // グループチャット一覧取得（null安全）
    fetch("http://localhost:8080/group_rooms", { credentials: "include" })
      .then(async res => {
        if (!res.ok) {
          const text = await res.text();
          throw new Error(`サーバーエラー: ${text}`);
        }
        const data = await res.json();
        if (!Array.isArray(data)) throw new Error("配列ではないレスポンス");
        setGroupRooms(data);
      })
      .catch(err => {
        console.error("❌ group_rooms取得失敗:", err);
        setGroupRooms([]);
      });
  }, [userId]);

  const restoreLastUser = async (users: User[]) => {
    const lastId = localStorage.getItem(`lastSelectedUserId_user${userId}`);
    if (!lastId) return;
    const found = users.find((u) => u.id === Number(lastId));
    if (!found) return;
    setSelectedUser(found);
    try {
      const roomRes = await fetch(`http://localhost:8080/room?user_id=${found.id}`, { credentials: "include" });
      const { room_id } = await roomRes.json();
      setRoomId(room_id);
      const msgRes = await fetch(`http://localhost:8080/messages?room_id=${room_id}`, { credentials: "include" });
      const msgs = await msgRes.json();
      setMessages(msgs || []);
    } catch (err) {
      console.error("復元失敗:", err);
    }
  };

  useEffect(() => {
    if (users.length > 0 && userId !== null) {
      restoreLastUser(users);
    }
  }, [users, userId]);

  useEffect(() => {
    if (messageEndRef.current) {
      messageEndRef.current.scrollIntoView({ behavior: "smooth" });
    }
  }, [messages]);

  const handleUserClick = async (user: User) => {
    setSelectedUser(user);
    localStorage.setItem(`lastSelectedUserId_user${userId}`, user.id.toString());
    const res = await fetch(`http://localhost:8080/room?user_id=${user.id}`, { credentials: "include" });
    const data = await res.json();
    setRoomId(data.room_id);
    const messageRes = await fetch(`http://localhost:8080/messages?room_id=${data.room_id}`, { credentials: "include" });
    const messageData = await messageRes.json();
    setMessages(messageData || []);
  };

  const handleSendMessage = async () => {
    if (!messageText.trim() || userId == null || roomId == null || !socket) return;
    const msg = {
      sender_id: userId,
      receiver_id: selectedUser?.id,
      room_id: roomId,
      content: messageText.trim(),
    };
    socket.send(JSON.stringify(msg));
    setMessages(prev => [...prev, { sender_id: userId, content: messageText }]);
    setMessageText("");
  };

  const handleLogout = async () => {
    await fetch("http://localhost:8080/logout", { method: "POST", credentials: "include" });
    router.push("/login");
  };

  return (
    <div style={{ display: "flex", height: "100vh" }}>
      {/* サイドバー */}
      <div style={{ width: "220px", borderRight: "1px solid #ccc", padding: "1rem", display: "flex", flexDirection: "column" }}>
        <button
          onClick={() => router.push("/group/create")}
          style={{ marginBottom: "1rem", padding: "0.4rem 0.6rem", backgroundColor: "#3498db", color: "white", border: "none", borderRadius: "4px", cursor: "pointer" }}>
          ＋ グループ作成
        </button>

        <h3>グループチャット</h3>
        {Array.isArray(groupRooms) && groupRooms.map(room => (
          <div key={room.id} style={{ padding: "0.5rem", cursor: "pointer", background: roomId === room.id ? "#eee" : "" }}
            onClick={async () => {
              setSelectedUser(null);
              setRoomId(room.id);
              const res = await fetch(`http://localhost:8080/messages?room_id=${room.id}`, { credentials: "include" });
              const data = await res.json();
              setMessages(data || []);
            }}
          >
            {room.room_name || `ルーム ${room.id}`}
          </div>
        ))}

        <h3 style={{ marginTop: "1rem" }}>ユーザー一覧</h3>
        {users.map(user => (
          <div key={user.id} style={{ padding: "0.5rem", cursor: "pointer", background: selectedUser?.id === user.id ? "#eee" : "" }}
            onClick={() => handleUserClick(user)}>
            {user.username}
          </div>
        ))}
      </div>

      {/* メイン画面 */}
      <div style={{ flex: 1, display: "flex", flexDirection: "column" }}>
        {/* ログアウトボタン */}
        <div style={{ padding: "1rem", textAlign: "right" }}>
          <button onClick={handleLogout} style={{ backgroundColor: "#e74c3c", color: "white", padding: "0.5rem 1rem", border: "none", borderRadius: "4px", cursor: "pointer" }}>
            ログアウト
          </button>
        </div>

        {/* チャット */}
        <div style={{ padding: "1rem", flex: 1 }}>
          {roomId ? (
            <>
              <h3>{selectedUser ? `${selectedUser.username} とのチャット` : "グループチャット"}</h3>
              <div style={{ height: "300px", overflowY: "scroll", display: "flex", flexDirection: "column", border: "1px solid #ccc", marginBottom: "1rem", padding: "0.5rem" }}>
                {messages.map((msg, i) => (
                  <div key={i} className={`${styles.message} ${msg.sender_id === userId ? styles.myMessage : styles.otherMessage}`}>
                    {msg.sender_id === userId ? "自分" : "相手"}: {msg.content}
                  </div>
                ))}
                <div ref={messageEndRef}></div>
              </div>
              <input
                type="text"
                value={messageText}
                onChange={(e) => setMessageText(e.target.value)}
                style={{ width: "80%" }}
                placeholder="メッセージを入力"
              />
              <button onClick={handleSendMessage}>送信</button>
            </>
          ) : (
            <p>チャットルームを選択してください</p>
          )}
        </div>
      </div>
    </div>
  );
}
