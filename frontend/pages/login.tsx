'use client';

import { useState } from "react";
import { useRouter } from "next/navigation";

export default function Login() {
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [message, setMessage] = useState("");
  const router = useRouter();

  const handleLogin = async (e: React.FormEvent) => {
    e.preventDefault();
    console.log("🚀 handleLogin 実行開始");

    const res = await fetch("http://localhost:8080/login", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      credentials: "include",
      body: JSON.stringify({
        username,
        password_hash: password,
      }),
    });

    console.log("📡 レスポンスステータス:", res.status);

    if (res.ok) {
      console.log("✅ ログイン成功");
      setMessage("✅ ログイン成功！チャット画面に移動します...");
      setTimeout(() => {
        router.push("/chat");
      }, 1000);
    } else {
      const errorText = await res.text();
      console.error("❌ ログイン失敗:", errorText);
      setMessage(`❌ ログイン失敗: ${errorText}`);
    }
  };

  return (
    <div style={{ padding: "2rem", maxWidth: "400px", margin: "0 auto" }}>
      <h1>ログイン</h1>
      <form onSubmit={handleLogin}>
        <div>
          <label>ユーザー名: </label>
          <input
            type="text"
            value={username}
            onChange={(e) => setUsername(e.target.value)}
            required
          />
        </div>
        <div>
          <label>パスワード: </label>
          <input
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            required
          />
        </div>
        <button type="submit">ログイン</button>
      </form>

      {message && (
        <p style={{ marginTop: "1rem", color: message.includes("成功") ? "green" : "red" }}>
          {message}
        </p>
      )}
    </div>
  );
}
