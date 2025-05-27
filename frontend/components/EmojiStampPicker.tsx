import { useState } from "react";
import Picker from '@emoji-mart/react';
import data from '@emoji-mart/data';

type Props = {
  onEmojiSelect: (emoji: string) => void;
  onStampSelect: (url: string) => void;
};

const stamps = [
  "https://placehold.co/80x80",
  "https://placebear.com/80/80",
  "https://placekitten.com/80/80"
];

export default function EmojiStampPicker({ onEmojiSelect, onStampSelect }: Props) {
  const [show, setShow] = useState(false);
  const [activeTab, setActiveTab] = useState<"emoji" | "stamp">("emoji");

  return (
    <div style={{ position: "relative", display: "inline-block" }}>
      <button onClick={() => setShow(prev => !prev)} style={{ marginRight: "8px" }}>😀</button>

      {show && (
        <div
          style={{
            position: "absolute",
            zIndex: 1000,
            background: "white",
            padding: "0.5rem",
            border: "1px solid #ccc",
            borderRadius: "0.5rem",
            top: "2.5rem",
            left: 0,
            width: "500px", // 横幅広げる
            maxHeight: "300px", // 縦幅短くする
            overflowY: "auto",
            boxShadow: "0 4px 12px rgba(0,0,0,0.1)"
          }}
        >
          {/* タブ切り替え */}
          <div style={{ display: "flex", marginBottom: "0.5rem" }}>
            <button
              onClick={() => setActiveTab("emoji")}
              style={{
                flex: 1,
                padding: "0.5rem",
                backgroundColor: activeTab === "emoji" ? "#3498db" : "#eee",
                color: activeTab === "emoji" ? "white" : "black",
                border: "none",
                borderRadius: "0.5rem 0 0 0.5rem",
                cursor: "pointer"
              }}
            >
              絵文字
            </button>
            <button
              onClick={() => setActiveTab("stamp")}
              style={{
                flex: 1,
                padding: "0.5rem",
                backgroundColor: activeTab === "stamp" ? "#3498db" : "#eee",
                color: activeTab === "stamp" ? "white" : "black",
                border: "none",
                borderRadius: "0 0.5rem 0.5rem 0",
                cursor: "pointer"
              }}
            >
              スタンプ
            </button>
          </div>

          {/* 絵文字 */}
          {activeTab === "emoji" && (
  <Picker
    data={data}
    onEmojiSelect={(emoji: any) => {
      onEmojiSelect(emoji.native);
      setShow(false);
    }}
    theme="light"
    previewPosition="none"
  />
)}

          {/* スタンプ */}
          {activeTab === "stamp" && (
            <div
              style={{
                display: "flex",
                flexWrap: "wrap",
                gap: "8px",
                justifyContent: "flex-start",
                maxHeight: "200px",
                overflowY: "auto",
                padding: "0.5rem"
              }}
            >
              {stamps.map((url, i) => (
                <img
                  key={i}
                  src={url}
                  alt={`stamp-${i}`}
                  style={{
                    width: "80px",   // サイズはそのまま
                    height: "80px",
                    objectFit: "cover",
                    cursor: "pointer",
                    borderRadius: "6px",
                    boxShadow: "0 2px 4px rgba(0,0,0,0.1)"
                  }}
                  onClick={() => {
                    onStampSelect(url);
                    setShow(false);
                  }}
                />
              ))}
            </div>
          )}
        </div>
      )}
    </div>
  );
}
