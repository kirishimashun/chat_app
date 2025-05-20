// 例: GroupCreateForm.tsx

'use client';
import { useEffect, useState } from 'react';

type User = {
  id: number;
  username: string;
};

export default function GroupCreateForm({ onRoomCreated }: { onRoomCreated: (roomId: number) => void }) {
  const [groupName, setGroupName] = useState('');
  const [users, setUsers] = useState<User[]>([]);
  const [selectedUserIds, setSelectedUserIds] = useState<number[]>([]);

  useEffect(() => {
    fetch('http://localhost:8080/users', { credentials: 'include' })
      .then(res => res.json())
      .then(setUsers)
      .catch(console.error);
  }, []);

  const toggleUser = (id: number) => {
    setSelectedUserIds(prev =>
      prev.includes(id) ? prev.filter(uid => uid !== id) : [...prev, id]
    );
  };

  const handleCreate = async () => {
    if (!groupName.trim() || selectedUserIds.length === 0) {
      alert('グループ名と参加メンバーを指定してください');
      return;
    }

    const res = await fetch('http://localhost:8080/rooms', {
      method: 'POST',
      credentials: 'include',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ name: groupName, user_ids: selectedUserIds }),
    });

    if (!res.ok) {
      alert('ルーム作成に失敗しました');
      return;
    }

    const data = await res.json();
    onRoomCreated(data.room_id); // 呼び出し元に通知
  };

  return (
    <div style={{ padding: '1rem', border: '1px solid #ccc', marginTop: '1rem' }}>
      <h4>グループ作成</h4>
      <input
        placeholder="グループ名"
        value={groupName}
        onChange={e => setGroupName(e.target.value)}
        style={{ width: '100%', marginBottom: '0.5rem' }}
      />
      <div>
        {users.map(user => (
          <label key={user.id} style={{ display: 'block' }}>
            <input
              type="checkbox"
              checked={selectedUserIds.includes(user.id)}
              onChange={() => toggleUser(user.id)}
            />
            {user.username}
          </label>
        ))}
      </div>
      <button onClick={handleCreate} style={{ marginTop: '0.5rem' }}>
        グループ作成
      </button>
    </div>
  );
}
