import React from "react";

type Props = {
  onSelect: (url: string) => void;
};

// 仮画像（外部の無料画像サービスを使ってます）
const stamps = [
  "https://placekitten.com/80/80",
  "https://placebear.com/80/80",
  "https://placehold.co/80x80"
];

export default function StampPicker({ onSelect }: Props) {
  return (
    <div style={{ display: "flex", gap: "8px", flexWrap: "wrap", padding: "4px" }}>
      {stamps.map((url, i) => (
        <img
          key={i}
          src={url}
          alt={`stamp${i}`}
          style={{ width: "48px", height: "48px", cursor: "pointer", borderRadius: "6px" }}
          onClick={() => onSelect(url)}
        />
      ))}
    </div>
  );
}
