import { useEffect, useMemo, useState } from "react";
import { BlockNoteEditor } from "@blocknote/core";
import { useCreateBlockNote } from "@blocknote/react";
import { BlockNoteView } from "@blocknote/mantine";
import "@blocknote/mantine/style.css";

/**
 * Try to parse content as BlockNote JSON first, fall back to Markdown.
 */
function parseContent(content) {
  if (!content) return undefined;

  // Try BlockNote JSON
  try {
    const parsed = JSON.parse(content);
    if (Array.isArray(parsed) && parsed.length > 0 && parsed[0].type) {
      return parsed;
    }
  } catch {
    // not JSON
  }

  // Fall back to Markdown parsing
  try {
    const tmp = BlockNoteEditor.create();
    return tmp.tryParseMarkdownToBlocks(content);
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

// StreamingPreview renders the raw Markdown arriving from the assistant as
// an ordered list of fade-in spans. React's index-based keys ensure that
// already-mounted chunks do not re-animate as new chunks are appended —
// only the newest span runs the CSS animation on its initial mount.
// Markdown is shown as plain text during streaming (no live formatting);
// the authoritative BlockNote render kicks in on the `complete` event.
function StreamingPreview({ chunks }) {
  return (
    <div className="px-10 py-2 text-sm leading-relaxed whitespace-pre-wrap text-slate-700">
      {chunks.map((c, i) => (
        <span key={i} className="chunk-fade">{c}</span>
      ))}
    </div>
  );
}

export function PostContent({ post, content, streamingChunks, onContentChange }) {
  const blocks = useMemo(() => parseContent(content), [content]);
  const [wordCount, setWordCount] = useState(0);
  const isStreaming = Array.isArray(streamingChunks) && streamingChunks.length > 0;

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
            {isStreaming ? (
              <StreamingPreview chunks={streamingChunks} />
            ) : (
              <EditorView
                key={content || "empty"}
                initialContent={blocks}
                onChange={onContentChange}
                onWordCount={setWordCount}
              />
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
