import { useEffect, useRef, useCallback } from "react";
import { useCreateBlockNote } from "@blocknote/react";
import { BlockNoteView } from "@blocknote/mantine";
import type { Block } from "@blocknote/core";
import "@blocknote/mantine/style.css";
import "./blocknote-theme.css";

type ContentPieceEditorProps = {
  initialContent: string;
  onContentChange: (content: string) => void;
};

const DEFAULT_CONTENT: Block[] = [
  {
    type: "heading",
    props: { level: 1 },
    content: [{ type: "text", text: "Untitled", styles: {} }],
  } as unknown as Block,
];

function parseBlocks(raw: string): Block[] {
  if (!raw || raw.trim() === "") return DEFAULT_CONTENT;
  try {
    const parsed = JSON.parse(raw);
    if (Array.isArray(parsed) && parsed.length > 0) return parsed;
  } catch {
    // not valid JSON — ignore
  }
  return DEFAULT_CONTENT;
}

export function ContentPieceEditor({
  initialContent,
  onContentChange,
}: ContentPieceEditorProps) {
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const editor = useCreateBlockNote({
    initialContent: parseBlocks(initialContent),
  });

  const handleChange = useCallback(() => {
    if (timerRef.current) clearTimeout(timerRef.current);
    timerRef.current = setTimeout(() => {
      const blocks = editor.document;
      onContentChange(JSON.stringify(blocks));
    }, 500);
  }, [editor, onContentChange]);

  useEffect(() => {
    return () => {
      if (timerRef.current) clearTimeout(timerRef.current);
    };
  }, []);

  return <BlockNoteView editor={editor} onChange={handleChange} theme="light" />;
}
