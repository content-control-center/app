import { useEffect, useRef, useState } from "react";
import {
  diffBlocks,
  groupListItems,
  parseMarkdownToStreamingBlocks,
} from "../lib/streaming-blocks.js";

/**
 * Render a streaming Markdown response as a live BlockNote-like preview
 * with per-chunk fade-in animation on the currently-writing block.
 *
 * Older blocks are frozen (committed text, no animation). Inline
 * Markdown syntax (bold / italic / link) is intentionally NOT rendered
 * during streaming — see Path B in the plan doc. When the stream ends,
 * the parent swaps this preview out for the authoritative BlockNote
 * editor which handles inline styles.
 */
export function StreamingMarkdownPreview({ chunks }) {
  const [blocks, setBlocks] = useState([]);
  const keyCounter = useRef(0);

  useEffect(() => {
    if (!chunks || chunks.length === 0) {
      setBlocks([]);
      keyCounter.current = 0;
      return;
    }
    const md = chunks.join("");
    const parsed = parseMarkdownToStreamingBlocks(md);
    setBlocks((prev) => diffBlocks(prev, parsed, keyCounter));
  }, [chunks]);

  if (blocks.length === 0) {
    return null;
  }

  const grouped = groupListItems(blocks);

  return (
    <div className="streaming-md-preview px-10 py-2 text-sm text-slate-700 leading-relaxed">
      {grouped.map((entry) => {
        if (entry.kind === "list") {
          const ListTag = entry.listKind;
          return (
            <ListTag
              key={entry.key}
              className={entry.listKind === "ul" ? "list-disc pl-5 mb-2" : "list-decimal pl-5 mb-2"}
            >
              {entry.items.map((b) => (
                <BlockView key={b.key} block={b} />
              ))}
            </ListTag>
          );
        }
        return <BlockView key={entry.block.key} block={entry.block} />;
      })}
    </div>
  );
}

function BlockView({ block }) {
  const inner = (
    <>
      {block.committed}
      {block.pending.map((c) => (
        <span key={c.key} className="chunk-fade">
          {c.text}
        </span>
      ))}
    </>
  );

  switch (block.type) {
    case "heading": {
      const level = Math.min(Math.max(block.level || 1, 1), 3);
      if (level === 1) return <h1 className="text-2xl font-bold mb-2">{inner}</h1>;
      if (level === 2) return <h2 className="text-xl font-bold mb-2">{inner}</h2>;
      return <h3 className="text-lg font-semibold mb-2">{inner}</h3>;
    }
    case "bulletListItem":
    case "numberedListItem":
      return <li className="mb-1">{inner}</li>;
    case "quote":
      return (
        <blockquote className="border-l-4 border-slate-300 pl-3 italic my-2">
          {inner}
        </blockquote>
      );
    case "codeBlock":
      return (
        <pre className="bg-slate-100 rounded p-2 text-xs font-mono overflow-x-auto my-2">
          {inner}
        </pre>
      );
    default:
      return <p className="mb-2">{inner}</p>;
  }
}
