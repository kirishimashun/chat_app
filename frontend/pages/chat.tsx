"use client";
import { useEffect, useState, useRef } from "react";
import { useRouter } from "next/navigation";
import EmojiStampPicker from "../components/EmojiStampPicker";

type User = { id: number; username: string };
type Message = {
  id: number;
  room_id: number;
  sender_id: number;
  content: string;
  read_at?: string | null;
  reactions?: { user_id: number; emoji: string }[];
};
type RoomInfo = { id: number; room_name: string; is_group: boolean };

export default function ChatPage() {
  const [users, setUsers] = useState<User[]>([]);
  const [selectedUser, setSelectedUser] = useState<User | null>(null);
  const [messages, setMessages] = useState<Message[]>([]);
  const [messageText, setMessageText] = useState("");
  const [userId, setUserId] = useState<number | null>(null);
  const [editingMessageId, setEditingMessageId] = useState<number | null>(null);
  const [editingText, setEditingText] = useState<string>("");
　const [menuOpenMessageId, setMenuOpenMessageId] = useState<number | null>(null);
  const menuRef = useRef<HTMLDivElement | null>(null);
  const [roomId, setRoomId] = useState<number | null>(null);
  const [userList, setUserList] = useState<{ id: number; name: string }[]>([]);

  const [roomMembers, setRoomMembers] = useState<User[]>([]);
  const [socket, setSocket] = useState<WebSocket | null>(null);
  const [groupRooms, setGroupRooms] = useState<RoomInfo[]>([]);
  const [previewUrl, setPreviewUrl] = useState<string | null>(null);
  const [activeReactionMessageId, setActiveReactionMessageId] = useState<number | null>(null);
  const router = useRouter();
  const messageEndRef = useRef<HTMLDivElement | null>(null);
  const [unreadCounts, setUnreadCounts] = useState<{ [roomId: number]: number }>({});
  const [userRoomMap, setUserRoomMap] = useState<{ [userId: number]: number }>({});
 const [mentionList, setMentionList] = useState<
  { from: number; room_id: number; message: string }[]
>([]);

  const [mentionOpen, setMentionOpen] = useState(false);


  const markAllAsRead = async (roomId: number) => {
    await fetch("http://localhost:8080/messages/read", {
      method: "POST",
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ room_id: roomId }),
    });
  };

  const markSingleAsRead = async (messageId: number) => {
  await fetch("http://localhost:8080/messages/read", {
    method: "POST",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ message_id: messageId }),
  });

  // ✅ 既読処理後、未読数を更新
  const res = await fetch("http://localhost:8080/unread_counts", {
    credentials: "include",
  });
  const data = await res.json();
  setUnreadCounts(data);
};



  const fetchRoomMap = async (users: User[]) => {
  const map: { [userId: number]: number } = {};
  for (const user of users) {
    try {
      const res = await fetch(`http://localhost:8080/room?user_id=${user.id}`, {
        credentials: "include",
      });
      const data = await res.json();
      if (data.room_id) {
        map[user.id] = data.room_id;
      }
    } catch (err) {
      console.error(`room取得失敗: user_id=${user.id}`, err);
    }
  }
  setUserRoomMap(map);
};


  const openRoomAndRead = async (targetRoomId: number) => {
  setRoomId(targetRoomId);
  await markAllAsRead(targetRoomId);

  const res = await fetch(`http://localhost:8080/messages?room_id=${targetRoomId}`, { credentials: "include" });
  const data = await res.json();
  setMessages(Array.isArray(data) ? data : data.messages || []);

  const memberRes = await fetch(`http://localhost:8080/room/members?room_id=${targetRoomId}`, { credentials: "include" });
  const memberData = await memberRes.json();
  setRoomMembers(Array.isArray(memberData) ? memberData : []);

  // ✅ 既読後、未読数を確実に再取得（バッジ消える）
  try {
  const unreadRes = await fetch("http://localhost:8080/unread_counts", { credentials: "include" });
  const unreadData = await unreadRes.json();
  setUnreadCounts(unreadData);

  // ✅ 自分にも未読数更新通知（バッジを消す）
  if (
  socket &&
  socket.readyState === WebSocket.OPEN &&
  roomId != null
) {
  socket.send(JSON.stringify({
    type: "read",
    message_id: -1,
    read_at: new Date().toISOString(),
  }));
}


} catch (err) {
  console.error("❌ 未読数の取得に失敗", err);
}

};


  const restoreLastUser = async (users: User[]) => {
    const lastId = localStorage.getItem(`lastSelectedUserId_user${userId}`);
    if (!lastId) return;
    const found = users.find(u => u.id === Number(lastId));
    if (!found) return;
    setSelectedUser(found);
    const res = await fetch(`http://localhost:8080/room?user_id=${found.id}`, { credentials: "include" });
    const { room_id } = await res.json();
    await openRoomAndRead(room_id);
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

    ws.onmessage = async (event) => {
  const data = JSON.parse(event.data);

  if (data.type === "message") {
  const msgRoomId = Number(data.room_id);
  if (isNaN(msgRoomId) || msgRoomId !== roomId) return; // ← ここで厳格にroom_idを確認

  const msg = {
    id: data.id,
    room_id: msgRoomId,
    sender_id: data.sender_id,
    content: data.content,
    read_at: data.read_at ?? null,
  };

  setMessages((prev) => [...prev, msg]);
  if (msg.sender_id !== userId) markSingleAsRead(msg.id);

    fetch("http://localhost:8080/unread_counts", { credentials: "include" })
      .then(res => res.json())
      .then(data => setUnreadCounts(data));

} else if (data.type === "read") {
  const id = Number(data.message_id);
  if (!isNaN(id)) {
    setMessages((prev) =>
      prev.map((m) => {
        // すでに read_at が存在していればそのまま（上書きしない）
        if (m.id === id && !m.read_at) {
          return { ...m, read_at: data.read_at };
        }
        return m;
      })
    );
  }

  } else if (data.type === "reaction") {
    const id = Number(data.message_id);
    if (!isNaN(id)) {
      setMessages((prev) =>
        prev.map((m) => {
          if (m.id !== id) return m;
          const prevReactions = m.reactions || [];
          const filtered = prevReactions.filter(
            (r) => r.user_id !== data.user_id
          );
          return {
            ...m,
            reactions: [...filtered, { user_id: data.user_id, emoji: data.emoji }],
          };
        })
      );
    }
  } else if (data.type === "edit") {
    const id = Number(data.message_id);
    if (!isNaN(id)) {
      setMessages((prev) =>
        prev.map((m) => (m.id === id ? { ...m, content: data.content } : m))
      );
    }
  } else if (data.type === "delete") {
    const id = Number(data.message_id);
    if (!isNaN(id)) {
      setMessages((prev) =>
        prev.map((m) =>
          m.id === id ? { ...m, content: "このメッセージは削除されました" } : m
        )
      );
    }
      // これ ↓ に置き換えてください
} else if (data.type === "unread") {
  const roomId = data.room_id;
  const count = data.count;

  const updatedCounts: { [roomId: number]: number } = { [roomId]: count };

  Object.entries(userRoomMap).forEach(([uid, rid]) => {
    if (rid === roomId) {
      updatedCounts[rid] = count;
    }
  });

  setUnreadCounts(prev => ({
    ...prev,
    ...updatedCounts,
  }));

    } else if (data.type === "mention") {
      setMentionList((prev) => [...prev, {
        from: data.from,
        room_id: data.room_id,
        message: data.message,
      }]);
    }
  };
  ws.onclose = () => console.warn("WebSocket closed");
  ws.onerror = (err) => console.error("WebSocket error", err);
  return () => ws.close();
}, [userId, roomId]);

  useEffect(() => {
    if (!userId) return;
    fetch("http://localhost:8080/users", { credentials: "include" })
      .then(res => res.json())
      .then(data => {
        setUsers(data);
      });
      fetch("http://localhost:8080/unread_counts", { credentials: "include" })
  .then(res => res.json())
  .then(data => setUnreadCounts(data));

  }, [userId]);

  useEffect(() => {
  if (!userId) return;
  fetch("http://localhost:8080/users", { credentials: "include" })
    .then((res) => res.json())
    .then((data) => {
      setUserList(data.map((u: User) => ({ id: u.id, name: u.username })));
    })
    .catch((err) => console.error("ユーザー取得失敗", err));
}, [userId]);


  useEffect(() => {
  const handleClickOutside = (e: MouseEvent) => {
    if (menuRef.current && !menuRef.current.contains(e.target as Node)) {
      setMenuOpenMessageId(null);
    }
  };
  document.addEventListener("mousedown", handleClickOutside);
  return () => document.removeEventListener("mousedown", handleClickOutside);
}, []);


  useEffect(() => {
    if (!userId) return;
    fetch("http://localhost:8080/group_rooms", { credentials: "include" })
      .then(res => res.json())
      .then(data => Array.isArray(data) ? setGroupRooms(data) : setGroupRooms([]));
  }, [userId]);

  const toggleReaction = async (messageId: number, emoji: string) => {
    await fetch("http://localhost:8080/reactions", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      credentials: "include",
      body: JSON.stringify({ message_id: messageId, emoji })
    });
  };

  
  const handleEditMessage = async (id: number, content: string) => {
    const res = await fetch(`http://localhost:8080/messages/edit?id=${id}`, {
      method: "PUT",
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ content })
    });
    if (res.ok) {
      setMessages(prev => prev.map(m => m.id === id ? { ...m, content } : m));
      setEditingMessageId(null);
    }
  };
  
  const handleHardDeleteMessage = async (id: number) => {
  const res = await fetch(`http://localhost:8080/messages/hard_delete?id=${id}`, {
    method: "DELETE",
    credentials: "include"
  });
  if (res.ok) {
    setMessages(prev => prev.filter(m => m.id !== id));
  }
};

  const handleDeleteMessage = async (id: number) => {
    const res = await fetch(`http://localhost:8080/messages/delete?id=${id}`, {
      method: "DELETE",
      credentials: "include"
    });
    if (res.ok) {
      setMessages(prev => prev.map(m => m.id === id ? { ...m, content: "このメッセージは削除されました" } : m));
    }
  };

const handleSendMessage = async () => {
  if (!messageText.trim() || userId == null || roomId == null || !socket) return;

  const content = messageText.trim();

  // 通常メッセージ送信
  const msg = {
    type: "message",
    sender_id: userId,
    receiver_id: selectedUser?.id,
    room_id: roomId,
    content: content,
  };
  socket.send(JSON.stringify(msg));

  // @メンション検出（例: @taro）
  const mentionRegex = /@([\w\u3040-\u309F\u30A0-\u30FF\u4E00-\u9FFF]+)/g;
  const mentionedUsernames = [...content.matchAll(mentionRegex)].map(match => match[1]);

  mentionedUsernames.forEach(username => {
    const targetUser = users.find(u => u.username === username);
    if (targetUser) {
      const mention = {
        type: "mention",
        from: userId,
        to: targetUser.id,
        room_id: roomId,
        message: content,
      };
      socket.send(JSON.stringify(mention));
    }
  });

  setMessageText("");
};


  const handleImageUpload = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file || !socket || userId == null || roomId == null) return;
    const formData = new FormData();
    formData.append("image", file);
    const res = await fetch("http://localhost:8080/upload", { method: "POST", body: formData });
    const { url } = await res.json();
    const msg = {
      type: "message",
      sender_id: userId,
      receiver_id: selectedUser?.id,
      room_id: roomId,
      content: url,
    };
    socket.send(JSON.stringify(msg));
  };

  const handleUserClick = async (user: User) => {
    setSelectedUser(user);
    localStorage.setItem(`lastSelectedUserId_user${userId}`, user.id.toString());
    const res = await fetch(`http://localhost:8080/room?user_id=${user.id}`, { credentials: "include" });
    const data = await res.json();
    await openRoomAndRead(data.room_id);
  };
  const getUsernameById = (id: number): string => {
  if (id === userId) return "自分";
  const user = [...users, ...roomMembers].find((u) => u.id === id);
  return user?.username || `ユーザー${id}`;
};

  const renderMessages = () =>
  messages
    .filter((msg) => msg.room_id === roomId) // ← ここで現在のルームのみに絞る
    .map((msg, i) => {
  const isMyMessage = msg.sender_id === userId;
  const isReadByOther = isMyMessage && typeof msg.read_at === "string" && msg.read_at !== "null";
  const isImageLike = msg.content.match(/^https?:\/\/.+\.(jpg|jpeg|png|gif|webp|svg)$/i)
    || msg.content.includes("/static/")
    || msg.content.includes("placebear.com")
    || msg.content.includes("placekitten.com");

  return (
    <div
      key={i}
      style={{
        display: "flex",
        flexDirection: "column",
        alignItems: isMyMessage ? "flex-end" : "flex-start",
        marginBottom: "12px",
        position: "relative",
      }}
      onClick={() => !isMyMessage && setActiveReactionMessageId(msg.id)}
    >
       {!isImageLike && (
      <p
        style={{
          fontSize: "0.8rem",
          fontWeight: "bold",
          marginBottom: "2px",
          cursor: "pointer",
          color: "blue",
          alignSelf: isMyMessage ? "flex-end" : "flex-start",
        }}
        onClick={(e) => {
          e.stopPropagation(); // 他のクリックイベントとの干渉防止
          const name = getUsernameById(msg.sender_id);
          setMessageText((prev) => prev + `@${name} `);
        }}
      >
        {getUsernameById(msg.sender_id)}
      </p>
    )}
      <div style={{ position: "relative", display: "inline-block",overflow: "visible", }}>
        {isImageLike ? (
          <>
            <img
              src={msg.content}
              alt="画像"
              style={{ width: "150px", borderRadius: "8px", cursor: "pointer" }}
              onClick={() => setPreviewUrl(msg.content)}
            />
          </>
        ) : (
          <>
            {editingMessageId === msg.id ? (
  <div style={{ display: "flex", gap: "0.5rem" }}>
    <input
      value={editingText}
      onChange={(e) => setEditingText(e.target.value)}
      style={{ padding: "0.3rem", borderRadius: "4px", border: "1px solid #ccc" }}
    />
    <button onClick={() => handleEditMessage(msg.id, editingText)}>保存</button>
    <button onClick={() => setEditingMessageId(null)}>キャンセル</button>
  </div>
) : (
  <div
    style={{
      backgroundColor: isMyMessage ? "#dff0ff" : "#f1f1f1",
      padding: "0.5rem",
      borderRadius: "1rem",
      maxWidth: "70%",
      whiteSpace: "pre-wrap",
    }}
  >
    {/* 👇 ここで msg.content のみ表示するよう変更 */}
    {msg.content}
  </div>
)}

          </>
        )}

        {/* ✅ 三点メニュー表示 */}
        {isMyMessage && editingMessageId !== msg.id && (
  <div style={{ position: "absolute", bottom: "0.2rem", right: "-1.5rem" }}>
    <button
      onClick={(e) => {
        e.stopPropagation();
        setMenuOpenMessageId(prev => prev === msg.id ? null : msg.id);
      }}
      style={{
        background: "none",
        border: "none",
        fontSize: "1.2rem",
        cursor: "pointer"
      }}
    >
      …
    </button>
    {menuOpenMessageId === msg.id && (
  <div
    ref={menuRef}
    onClick={(e) => e.stopPropagation()}
    style={{
      position: "absolute",
      bottom: "2rem",
      right: 0,
      backgroundColor: "white",
      border: "1px solid #ccc",
      borderRadius: "6px",
      boxShadow: "0 2px 8px rgba(0,0,0,0.1)",
      padding: "0.4rem",
      display: "flex",
      flexDirection: "column",
      gap: "0.3rem",
      zIndex: 1000,
      minWidth: "100px",
    }}
  >
    <button
      onClick={(e) => {
        e.stopPropagation();
        setEditingMessageId(msg.id);
        setEditingText(msg.content);
        setMenuOpenMessageId(null);
      }}
      style={{
        background: "#eee",
        border: "none",
        padding: "0.3rem 0.5rem",
        borderRadius: "4px",
        cursor: "pointer",
      }}
    >
      編集
    </button>
    <button
      onClick={(e) => {
        e.stopPropagation();
        handleDeleteMessage(msg.id);
        setMenuOpenMessageId(null);
      }}
      style={{
        background: "#fdd",
        border: "none",
        padding: "0.3rem 0.5rem",
        borderRadius: "4px",
        cursor: "pointer",
      }}
    >
      送信取消
    </button>
    <button
      onClick={(e) => {
        e.stopPropagation();
        handleHardDeleteMessage(msg.id);
        setMenuOpenMessageId(null);
      }}
      style={{
        background: "#faa",
        border: "none",
        padding: "0.3rem 0.5rem",
        borderRadius: "4px",
        cursor: "pointer",
      }}
    >
      削除
    </button>
  </div>
)}

  </div>
)}

        {isMyMessage && isReadByOther && (
          <div style={{
            position: "absolute",
            bottom: "0",
            left: "-36px",
            fontSize: "0.75rem",
            color: "gray"
          }}>既読</div>
        )}
      </div>

      {(msg.reactions ?? []).length > 0 && (
        <div style={{
          marginTop: "4px",
          alignSelf: isMyMessage ? "flex-end" : "flex-start",
          display: "flex",
          gap: "6px"
        }}>
          {msg.reactions!.map((r, idx) => (
            <span key={idx} style={{ fontSize: "1.2rem" }}>{r.emoji}</span>
          ))}
        </div>
      )}

      {!isMyMessage && activeReactionMessageId === msg.id && (
        <div style={{
          marginTop: "4px",
          display: "flex",
          gap: "4px",
          alignSelf: "flex-start"
        }}>
          {["👍", "❤️", "😂"].map(e => (
            <button key={e} onClick={(ev) => {
              ev.stopPropagation();
              toggleReaction(msg.id, e);
              setActiveReactionMessageId(null);
            }} style={{
              background: "#eee",
              borderRadius: "12px",
              padding: "2px 6px",
              border: "1px solid #ccc",
              cursor: "pointer"
            }}>{e}</button>
          ))}
        </div>
      )}
    </div>
  );
});

  useEffect(() => {
    if (messageEndRef.current) messageEndRef.current.scrollIntoView({ behavior: "smooth" });
  }, [messages]);

  return (
    <div style={{ display: "flex", height: "100vh" }}>
      <div style={{ width: "220px", borderRight: "1px solid #ccc", padding: "1rem", display: "flex", flexDirection: "column" }}>
        <button onClick={() => router.push("/group/create")} style={{ marginBottom: "1rem", padding: "0.4rem 0.6rem", backgroundColor: "#3498db", color: "white", border: "none", borderRadius: "4px", cursor: "pointer" }}>＋ グループ作成</button>
        <h3>グループチャット</h3>
        {groupRooms.map(room => (
  <div key={room.id}
    style={{ padding: "0.5rem", cursor: "pointer", background: roomId === room.id ? "#eee" : "", display: "flex", justifyContent: "space-between", alignItems: "center" }}
    onClick={async () => {
      setSelectedUser(null);
      await openRoomAndRead(room.id);
    }}>
    <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", width: "100%" }}>
  <span>{room.room_name || `ルーム ${room.id}`}</span>
  {unreadCounts[room.id] > 0 && (
    <span style={{
      background: "red",
      color: "white",
      borderRadius: "9999px",
      padding: "0.2rem 0.6rem",
      fontSize: "0.75rem",
      marginLeft: "0.5rem"
    }}>
      {unreadCounts[room.id]}
    </span>
  )}
</div>

  </div>
))}

        <h3 style={{ marginTop: "1rem" }}>ユーザー一覧</h3>
        {users.map(user => {
  const roomId = userRoomMap[user.id];
  const unread = unreadCounts[roomId] || 0;

  return (
    <div key={user.id}
      style={{
        padding: "0.5rem",
        cursor: "pointer",
        background: selectedUser?.id === user.id ? "#eee" : "",
        display: "flex",
        justifyContent: "space-between",
        alignItems: "center"
      }}
      onClick={() => handleUserClick(user)}>
      
      <span>{user.username}</span>

      {unread > 0 && (
        <span
          style={{
            background: "red",
            color: "white",
            borderRadius: "9999px",
            padding: "0.2rem 0.6rem",
            fontSize: "0.75rem",
            marginLeft: "0.5rem"
          }}
        >
          {unread}
        </span>
      )}
    </div>
  );
})}


      </div>
      <div style={{ flex: 1, display: "flex", flexDirection: "column" }}>
        <div style={{ padding: "1rem", textAlign: "right" }}>
          <button onClick={async () => {
            await fetch("http://localhost:8080/logout", { method: "POST", credentials: "include" });
            router.push("/login");
          }} style={{ backgroundColor: "#e74c3c", color: "white", padding: "0.5rem 1rem", border: "none", borderRadius: "4px", cursor: "pointer" }}>ログアウト</button>
        </div>
        <div style={{ position: "relative", textAlign: "right", padding: "0 1rem" }}>
  <button
    onClick={() => setMentionOpen(prev => !prev)}
    style={{ background: "none", border: "none", position: "relative", cursor: "pointer" }}
  >
    <span style={{ fontSize: "1.5rem" }}>🔔</span>
    {mentionList.length > 0 && (
      <span style={{
        position: "absolute",
        top: "-6px",
        right: "-6px",
        background: "red",
        color: "white",
        borderRadius: "50%",
        padding: "2px 6px",
        fontSize: "0.7rem"
      }}>
        {mentionList.length}
      </span>
    )}
  </button>

  {mentionOpen && (
    <div style={{
      position: "absolute",
      top: "2.5rem",
      right: 0,
      background: "white",
      border: "1px solid #ccc",
      borderRadius: "6px",
      boxShadow: "0 2px 8px rgba(0,0,0,0.1)",
      padding: "0.5rem",
      zIndex: 1000,
      width: "280px"
    }}>
      <strong>メンション通知</strong>
      <ul style={{ listStyle: "none", paddingLeft: 0, marginTop: "0.5rem" }}>
        {mentionList.map((m, idx) => (
          <li key={idx} style={{ marginBottom: "0.75rem", fontSize: "0.9rem" }}>
            📣 <strong>UserID: {m.from}</strong>：{m.message}<br />
            <button
              onClick={async () => {
                setMentionOpen(false);
                setSelectedUser(null);
                await openRoomAndRead(m.room_id);
              }}
              style={{
                fontSize: "0.75rem",
                marginTop: "0.2rem",
                background: "#eee",
                border: "none",
                padding: "0.2rem 0.4rem",
                borderRadius: "4px",
                cursor: "pointer",
              }}
            >
              チャットを開く
            </button>
          </li>
        ))}
      </ul>
    </div>
  )}
</div>

        <div style={{ padding: "1rem", flex: 1 }}>
          {roomId ? (
            <>
              <h3>{selectedUser ? `${selectedUser.username} とのチャット` : "グループチャット"}</h3>
              {roomId && !selectedUser && (
                <div style={{ marginBottom: "0.75rem", display: "flex", alignItems: "center" }}>
                  <strong style={{ marginRight: "0.5rem" }}>メンバー一覧：</strong>
                  <div style={{ display: "flex", flexWrap: "wrap", gap: "0.4rem" }}>
                    {roomMembers.map(member => (
                      <span key={member.id} style={{ background: "#eee", padding: "0.3rem 0.6rem", borderRadius: "1rem" }}>{member.username}</span>
                    ))}
                  </div>
                </div>
              )}
              <div style={{ height: "300px", overflowY: "scroll", display: "flex", flexDirection: "column", border: "1px solid #ccc", marginBottom: "1rem", padding: "0.5rem" }}>
                {renderMessages()}
                <div ref={messageEndRef}></div>
              </div>
              <input type="file" accept="image/*" onChange={handleImageUpload} style={{ marginBottom: "0.5rem" }} />
              <EmojiStampPicker
                onEmojiSelect={(emoji) => setMessageText((prev) => prev + emoji)}
                onStampSelect={(url) => {
                  if (!socket || !userId || !roomId) return;
                  const msg = {
                    type: "message",
                    sender_id: userId,
                    receiver_id: selectedUser?.id,
                    room_id: roomId,
                    content: url,
                  };
                  socket.send(JSON.stringify(msg));
                }}
              />
              <input type="text" value={messageText} onChange={e => setMessageText(e.target.value)} style={{ width: "80%" }} placeholder="メッセージを入力" />
              <button onClick={handleSendMessage}>送信</button>
            </>
          ) : (
            <p>チャットルームを選択してください</p>
          )}
        </div>
      </div>
      {previewUrl && (
        <div style={{ position: "fixed", top: 0, left: 0, width: "100vw", height: "100vh", backgroundColor: "rgba(0,0,0,0.7)", display: "flex", justifyContent: "center", alignItems: "center", zIndex: 1000 }} onClick={() => setPreviewUrl(null)}>
          <img src={previewUrl} style={{ maxWidth: "90%", maxHeight: "90%", borderRadius: "8px" }} />
        </div>
      )}
    </div>
  );
}
