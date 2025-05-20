// components/Sidebar.tsx
"use client";
import { useEffect, useState } from "react";
import Link from "next/link";
import GroupCreateForm from "./GroupCreateForm";

type ChatRoom = {
  id: number;
  room_name: string;
  is_group: boolean;
};

export default function Sidebar() {
  const [groupRooms, setGroupRooms] = useState<ChatRoom[]>([]);
  const [showForm, setShowForm] = useState(false);

  useEffect(() => {
    fetch("/group_rooms")
      .then((res) => {
        if (!res.ok) throw new Error("Failed to fetch");
        return res.json();
      })
      .then(setGroupRooms)
      .catch((err) => console.error("❌ グループ取得失敗", err));
  }, []);

  return (
    <aside className="w-64 bg-gray-100 px-4 pt-4 pb-8 h-[calc(100vh-64px)] overflow-y-auto">

      <button
        onClick={() => setShowForm(!showForm)}
        className="mb-4 px-4 py-2 bg-blue-500 text-white rounded hover:bg-blue-600 self-start"

      >
        ＋ グループ作成
      </button>

      {showForm && (
        <div className="mb-4 p-2 border rounded bg-white">
          <GroupCreateForm
            onRoomCreated={(roomId: number) => {
              setGroupRooms((prev) => [...prev, { id: roomId, room_name: `ルーム ${roomId}`, is_group: true }]);
              setShowForm(false);
            }}
          />
        </div>
      )}

      <h2 className="mt-2 mb-2 text-lg font-semibold text-gray-800">
        グループチャット
      </h2>
      <ul className="space-y-1 overflow-y-auto">
        {groupRooms.map((room) => (
          <li key={room.id}>
            <Link
              href={`/chat/${room.id}`}
              className="block px-3 py-2 rounded hover:bg-gray-200"
            >
              {room.room_name}
            </Link>
          </li>
        ))}
      </ul>
    </aside>
  );
}
