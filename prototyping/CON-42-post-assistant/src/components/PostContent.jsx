import { useCallback, useEffect, useMemo, useState } from "react";
import { BlockNoteEditor } from "@blocknote/core";
import { useCreateBlockNote } from "@blocknote/react";
import { BlockNoteView } from "@blocknote/mantine";
import "@blocknote/mantine/style.css";

/**
 * Try to parse description as BlockNote JSON first, fall back to Markdown.
 */
function parseContent(description) {
  if (!description) return undefined;

  // Try BlockNote JSON
  try {
    const parsed = JSON.parse(description);
    if (Array.isArray(parsed) && parsed.length > 0 && parsed[0].type) {
      return parsed;
    }
  } catch {
    // not JSON
  }

  // Fall back to Markdown parsing
  try {
    const tmp = BlockNoteEditor.create();
    return tmp.tryParseMarkdownToBlocks(description);
  } catch {
    return undefined;
  }
}

function getTextFromBlocks(blocks) {
  let text = "";
  for (const block of blocks) {
    if (block.content) {
      for (const inline of Array.isArray(block.content) ? block.content : []) {
        if (inline.type === "text") text += inline.text + " ";
      }
    }
    if (block.children?.length) text += getTextFromBlocks(block.children);
  }
  return text;
}

function countWords(blocks) {
  const text = getTextFromBlocks(blocks).trim();
  if (!text) return 0;
  return text.split(/\s+/).length;
}

function EditorView({ initialContent, onChange, onWordCount }) {
  const editor = useCreateBlockNote({ initialContent });

  useEffect(() => {
    onWordCount?.(countWords(editor.document));
  }, [editor]);

  useEffect(() => {
    const unsubscribe = editor.onChange(() => {
      onChange?.(editor.document);
      onWordCount?.(countWords(editor.document));
    });
    return unsubscribe;
  }, [editor, onChange, onWordCount]);

  return <BlockNoteView editor={editor} theme="light" />;
}

export function PostContent({ post, description, onContentChange }) {
  const blocks = useMemo(() => parseContent(description), [description]);
  const [wordCount, setWordCount] = useState(0);

  if (!post) {
    return (
      <div className="flex h-full items-center justify-center text-xs text-slate-400">
        Select a post to view its content
      </div>
    );
  }

  return (
    <div className="flex h-full flex-col overflow-hidden bg-slate-100 p-6">
      <div className="flex-1 overflow-y-auto">
        <div
          className="mx-auto max-w-2xl rounded bg-white shadow-md ring-1 ring-slate-200/60"
          style={{ minHeight: "100%" }}
        >
          {post.title && (
            <h1 className="text-lg font-bold text-slate-900 px-10 pt-8 pb-3 mb-0 border-b border-slate-100">
              {post.title}
            </h1>
          )}
          <div className="py-4">
            {blocks ? (
              <EditorView
                key={description}
                initialContent={blocks}
                onChange={onContentChange}
                onWordCount={setWordCount}
              />
            ) : (
              <p className="px-10 text-sm italic text-slate-400">No content yet</p>
            )}
          </div>
        </div>
      </div>
      <div className="shrink-0 border-t border-slate-200 bg-white px-6 py-1.5 text-[11px] text-slate-400">
        {wordCount} {wordCount === 1 ? "word" : "words"}
      </div>
    </div>
  );
}
