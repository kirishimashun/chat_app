import { useState } from "react";
import { Picker } from "emoji-mart";
import 'emoji-mart/css/emoji-mart.css';

type EmojiPickerProps = {
  onSelect: (emoji: string) => void;
};

export default function EmojiPicker({ onSelect }: EmojiPickerProps) {
  const [show, setShow] = useState(false);

  return (
    <div style={{ position: "relative", display: "inline-block" }}>
      <button onClick={() => setShow(prev => !prev)} style={{ marginRight: "8px" }}>😀</button>
      {show && (
        <div style={{ position: "absolute", zIndex: 1000 }}>
          <Picker
            onSelect={(emoji: any) => {
              onSelect(emoji.native);  // 😄 など
              setShow(false);
            }}
          />
        </div>
      )}
    </div>
  );
}
