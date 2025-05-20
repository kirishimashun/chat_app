// pages/group/create.tsx
'use client';
import { useRouter } from "next/navigation"; // ← 修正ポイント！
import GroupCreateForm from "../../components/GroupCreateForm";

export default function GroupCreatePage() {
  const router = useRouter();

  return (
    <div style={{ padding: "2rem", maxWidth: "600px", margin: "0 auto" }}>
      <h1 style={{ fontSize: "1.5rem", marginBottom: "1rem" }}>グループ作成</h1>

      <GroupCreateForm
        onRoomCreated={(roomId: number) => {
          router.push(`/chat?room_id=${roomId}`);
        }}
      />
    </div>
  );
}
