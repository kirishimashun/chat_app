"use client";
import { useEffect, useState, useRef } from "react";
import { useRouter } from "next/navigation";

type User = { id: number; username: string };
type Message = { id: number; room_id: number; sender_id: number; content: string; read_at?: string | null };
type RoomInfo = { id: number; room_name: string; is_group: boolean };

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

  const markAllAsRead = async (roomId: number) => {
    try {
      await fetch("http://localhost:8080/messages/read", {
        method: "POST",
        credentials: "include",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ room_id: roomId }),
      });
    } catch (err) {
      console.error("❌ markAllAsRead error:", err);
    }
  };

  const markSingleAsRead = async (messageId: number) => {
  try {
    await fetch("http://localhost:8080/messages/read", {
      method: "POST",
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ message_id: messageId }),
    });
  } catch (err) {
    console.error("❌ markSingleAsRead error:", err);
  }
};


  const restoreLastUser = async (users: User[]) => {
    const lastId = localStorage.getItem(`lastSelectedUserId_user${userId}`);
    if (!lastId) return;
    const found = users.find((u) => u.id === Number(lastId));
    if (!found) return;
    setSelectedUser(found);
    const res = await fetch(`http://localhost:8080/room?user_id=${found.id}`, { credentials: "include" });
    const { room_id } = await res.json();
    setRoomId(room_id);
    const msgRes = await fetch(`http://localhost:8080/messages?room_id=${room_id}`, { credentials: "include" });
    const msgs: Message[] = await msgRes.json();
    setMessages(msgs);
    await markAllAsRead(room_id);
  };

  useEffect(() => {
    fetch("http://localhost:8080/me", { credentials: "include" })
      .then(res => res.json())
      .then(data => setUserId(Number(data.user_id)))
      .catch(() => router.push("/login"));
  }, []);

  useEffect(() => {
    if (!userId || !roomId) return;

    const ws = new WebSocket("ws://localhost:8080/ws");
    ws.onopen = async () => {
      setSocket(ws);
      await markAllAsRead(roomId);
    };
    ws.onclose = () => console.warn("WebSocket closed");
    ws.onerror = err => console.error("WebSocket error", err);

ws.onmessage = async (event) => {
  const data = JSON.parse(event.data);
  console.log("💬 WebSocket 受信:", data);

  if (data.type === "message") {
    console.log("🧪 受信メッセージ:", data);

    if (typeof data.id !== "number" || typeof data.sender_id !== "number") {
      console.warn("⚠️ 不正なmessageを受信:", data);
      return;
    }

    const msg: Message = {
      id: data.id,
      room_id: data.room_id ?? roomId!,
      sender_id: data.sender_id,
      content: data.content,
      read_at: data.read_at ?? null,
    };

    setMessages(prev => [...prev, msg]);

    const isFromOtherUser = msg.sender_id !== userId;
    const isCurrentRoom = msg.room_id === roomId;

    if (isFromOtherUser && isCurrentRoom) {
      markSingleAsRead(msg.id);
    }
  } else if (data.type === "read") {
    console.log("📩 read受信:", data);

    const id = Number(data.message_id);
    if (!isNaN(id)) {
      setMessages(prev =>
        prev.map(m => m.id === id ? { ...m, read_at: data.read_at } : m)
      );
    }
  }
};


    return () => ws.close();
  }, [userId, roomId]);

  useEffect(() => {
    if (!userId) return;
    fetch("http://localhost:8080/users", { credentials: "include" })
      .then(res => res.json())
      .then(data => {
        setUsers(data);
        restoreLastUser(data);
      });
  }, [userId]);

  useEffect(() => {
    if (!userId) return;
    fetch("http://localhost:8080/group_rooms", { credentials: "include" })
      .then(res => res.json())
      .then(data => Array.isArray(data) ? setGroupRooms(data) : setGroupRooms([]));
  }, [userId]);

const handleSendMessage = async () => {
  if (!messageText.trim() || userId == null || roomId == null || !socket) return;

  const msg = {
    type: "message",
    sender_id: userId,
    receiver_id: selectedUser?.id,
    room_id: roomId,
    content: messageText.trim(),
  };

  socket.send(JSON.stringify(msg)); // ← WebSocket送信だけでOK
  setMessageText("");              // ← 自分で setMessages は不要にできる
};


  const handleUserClick = async (user: User) => {
    setSelectedUser(user);
    localStorage.setItem(`lastSelectedUserId_user${userId}`, user.id.toString());
    const res = await fetch(`http://localhost:8080/room?user_id=${user.id}`, { credentials: "include" });
    const data = await res.json();
    setRoomId(data.room_id);
    const msgRes = await fetch(`http://localhost:8080/messages?room_id=${data.room_id}`, { credentials: "include" });
    const msgs: Message[] = await msgRes.json();
    setMessages(msgs);
    await markAllAsRead(data.room_id);
  };

  const renderMessages = () => {
  return messages.map((msg, i) => {
    // ⚠️ 不正なメッセージオブジェクトをスキップ
    if (!msg || typeof msg.sender_id !== "number") {
      console.warn("⚠️ renderMessages内で不正なmsg:", msg);
      return null;
    }

    const isMyMessage = msg.sender_id === userId;
    const isReadByOther = isMyMessage && msg.read_at;

    return (
      <div key={i} style={{ display: "flex", justifyContent: isMyMessage ? "flex-end" : "flex-start", marginBottom: "8px" }}>
        {isMyMessage ? (
          <div style={{ display: "flex", flexDirection: "row", alignItems: "flex-end" }}>
            {isReadByOther && (
              <span style={{ fontSize: "0.75rem", color: "gray", marginRight: "4px" }}>既読</span>
            )}
            <div style={{ backgroundColor: "#dff0ff", padding: "0.5rem 0.8rem", borderRadius: "1rem", maxWidth: "70%" }}>
              自分: {msg.content}
            </div>
          </div>
        ) : (
          <div style={{ backgroundColor: "#f1f1f1", padding: "0.5rem 0.8rem", borderRadius: "1rem", maxWidth: "70%" }}>
            相手: {msg.content}
          </div>
        )}
      </div>
    );
  });
};


  useEffect(() => {
    if (messageEndRef.current) messageEndRef.current.scrollIntoView({ behavior: "smooth" });
  }, [messages]);

  return (
    <div style={{ display: "flex", height: "100vh" }}>
      <div style={{ width: "220px", borderRight: "1px solid #ccc", padding: "1rem", display: "flex", flexDirection: "column" }}>
        <button onClick={() => router.push("/group/create")} style={{ marginBottom: "1rem", padding: "0.4rem 0.6rem", backgroundColor: "#3498db", color: "white", border: "none", borderRadius: "4px", cursor: "pointer" }}>＋ グループ作成</button>
        <h3>グループチャット</h3>
        {groupRooms.map(room => (
          <div key={room.id} style={{ padding: "0.5rem", cursor: "pointer", background: roomId === room.id ? "#eee" : "" }}
            onClick={async () => {
              setSelectedUser(null);
              setRoomId(room.id);
              const res = await fetch(`http://localhost:8080/messages?room_id=${room.id}`, { credentials: "include" });
              const data = await res.json();
              setMessages(data || []);
              await markAllAsRead(room.id);
            }}>{room.room_name || `ルーム ${room.id}`}</div>
        ))}
        <h3 style={{ marginTop: "1rem" }}>ユーザー一覧</h3>
        {users.map(user => (
          <div key={user.id} style={{ padding: "0.5rem", cursor: "pointer", background: selectedUser?.id === user.id ? "#eee" : "" }}
            onClick={() => handleUserClick(user)}>
            {user.username}
          </div>
        ))}
      </div>

      <div style={{ flex: 1, display: "flex", flexDirection: "column" }}>
        <div style={{ padding: "1rem", textAlign: "right" }}>
          <button onClick={async () => {
            await fetch("http://localhost:8080/logout", { method: "POST", credentials: "include" });
            router.push("/login");
          }} style={{ backgroundColor: "#e74c3c", color: "white", padding: "0.5rem 1rem", border: "none", borderRadius: "4px", cursor: "pointer" }}>ログアウト</button>
        </div>
        <div style={{ padding: "1rem", flex: 1 }}>
          {roomId ? (
            <>
              <h3>{selectedUser ? `${selectedUser.username} とのチャット` : "グループチャット"}</h3>
              <div style={{ height: "300px", overflowY: "scroll", display: "flex", flexDirection: "column", border: "1px solid #ccc", marginBottom: "1rem", padding: "0.5rem" }}>
                {renderMessages()}
                <div ref={messageEndRef}></div>
              </div>
              <input type="text" value={messageText} onChange={(e) => setMessageText(e.target.value)} style={{ width: "80%" }} placeholder="メッセージを入力" />
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
