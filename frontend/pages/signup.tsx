'use client';

import { useState } from 'react';
import { useRouter } from 'next/navigation';

export default function Signup() {
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [message, setMessage] = useState('');
  const router = useRouter();

  const handleSubmit = async (event: React.FormEvent) => {
    event.preventDefault();

    try {
      const response = await fetch("http://localhost:8080/signup", {
        method: "POST",
        headers: {
          "Content-Type": "application/json"
        },
        body: JSON.stringify({
          username,
          password_hash: password
        })
      });

      if (response.ok) {
        const data = await response.json();
        console.log("User created:", data);
        setMessage("✅ 登録が完了しました！ログイン画面に移動します...");
        setTimeout(() => {
          router.push('/login');
        }, 1000);
      } else {
        const errText = await response.text();
        setMessage("❌ 登録に失敗しました: " + errText);
      }
    } catch (error: unknown) {
      if (error instanceof Error) {
        setMessage("❌ 通信エラー: " + error.message);
      } else {
        setMessage("❌ 通信エラー: 不明なエラーが発生しました");
      }
    }
  };

  return (
    <div style={{ padding: "2rem", maxWidth: "400px", margin: "0 auto" }}>
      <h1>サインアップ</h1>
      <form onSubmit={handleSubmit}>
        <div>
          <label>ユーザー名: </label>
          <input
            type="text"
            value={username}
            onChange={(e) => setUsername(e.target.value)}
            required
            style={{ width: "100%", marginBottom: "1rem" }}
          />
        </div>
        <div>
          <label>パスワード: </label>
          <input
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            required
            style={{ width: "100%", marginBottom: "1rem" }}
          />
        </div>
        <button type="submit">登録</button>
      </form>

      {message && (
        <p style={{ marginTop: "1rem", color: message.includes("✅") ? "green" : "red" }}>
          {message}
        </p>
      )}
    </div>
  );
}
