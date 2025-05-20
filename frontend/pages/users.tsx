import { useEffect, useState } from "react";

type User = {
  id: number;
  username: string;
};

export default function UsersList() {
  const [users, setUsers] = useState<User[]>([]);
  const [error, setError] = useState("");

  useEffect(() => {
    const fetchUsers = async () => {
      try {
        const res = await fetch("http://localhost:8080/users", {
          credentials: "include", 
        });
        if (!res.ok) {
          throw new Error("ユーザー取得に失敗しました");
        }
        const data = await res.json();
        setUsers(data);
      } catch (err) {
        setError((err as Error).message);
      }
    };

    fetchUsers();
  }, []);

  return (
    <div style={{ padding: "2rem" }}>
      <h1>ユーザー一覧</h1>
      {error && <p style={{ color: "red" }}>{error}</p>}
      <ul>
        {users.map((user) => (
          <li key={user.id}>{user.username}</li>
        ))}
      </ul>
    </div>
  );
}
