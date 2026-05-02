import { BlockNoteEditor } from "@blocknote/core";

/**
 * Streaming Markdown → block tree with per-chunk animation support.
 *
 * BlockNote's tryParseMarkdownToBlocks parses an accumulated Markdown
 * string on every delta. We post-process the result into a lightweight
 * `StreamingBlock` shape that also tracks which tail portion of the
 * current last block is "pending" (not yet animated in). Older blocks
 * are frozen — their text is committed and doesn't re-animate.
 */

// Walk a BlockNote block's inline `content` and return the concatenated
// plain text. Handles `text` and `link` inline nodes; unknown shapes are
// skipped.
export function blockText(block) {
  return inlineText(block.content || []);
}

function inlineText(inlines) {
  if (!Array.isArray(inlines)) return "";
  let out = "";
  for (const ic of inlines) {
    if (!ic) continue;
    if (ic.type === "text" && typeof ic.text === "string") {
      out += ic.text;
    } else if (ic.type === "link") {
      out += inlineText(ic.content || []);
    }
  }
  return out;
}

// Build a shared editor once for parsing — cheaper than creating one on
// every delta. tryParseMarkdownToBlocks is pure with respect to its input.
let parserEditor = null;
function parser() {
  if (!parserEditor) parserEditor = BlockNoteEditor.create();
  return parserEditor;
}

// Parse accumulated Markdown and return a flat array of
// { type, level, text } entries. Level is set only for headings.
export function parseMarkdownToStreamingBlocks(md) {
  try {
    const blocks = parser().tryParseMarkdownToBlocks(md);
    if (!Array.isArray(blocks)) return [];
    return blocks.map((b) => ({
      type: b.type,
      level: b.props?.level,
      text: blockText(b),
    }));
  } catch {
    return [];
  }
}

/**
 * Diff previous StreamingBlock[] against freshly parsed blocks.
 *
 * Rules:
 *  - All blocks except the last are frozen: committed = full text,
 *    pending = [].
 *  - The last block animates: text prefix that already existed stays
 *    in committed + previous pending; any newly appended suffix becomes
 *    a new chunk appended to pending.
 *  - If a block's type changes or its text no longer prefixes the old
 *    text (rare — Markdown restructured), reset that block's chunks.
 *
 * counterRef is a React ref whose `.current` value is a monotonic int
 * used to mint stable keys. The function mutates counterRef.current.
 */
export function diffBlocks(prev, parsed, counterRef) {
  const result = [];
  const n = parsed.length;
  const mint = () => `k${counterRef.current++}`;

  for (let i = 0; i < n; i++) {
    const p = parsed[i];
    const prevBlock = prev[i];
    const isLast = i === n - 1;

    if (!isLast) {
      // Freeze: collapse any pending chunks into committed.
      if (prevBlock && prevBlock.type === p.type) {
        result.push({
          key: prevBlock.key,
          type: p.type,
          level: p.level,
          committed: p.text,
          pending: [],
        });
      } else {
        result.push({
          key: mint(),
          type: p.type,
          level: p.level,
          committed: p.text,
          pending: [],
        });
      }
      continue;
    }

    // Last block — animated.
    if (prevBlock && prevBlock.type === p.type) {
      const oldText =
        prevBlock.committed +
        prevBlock.pending.map((c) => c.text).join("");
      if (p.text.startsWith(oldText)) {
        const appended = p.text.slice(oldText.length);
        result.push({
          key: prevBlock.key,
          type: p.type,
          level: p.level,
          committed: prevBlock.committed,
          pending: appended
            ? [...prevBlock.pending, { key: mint(), text: appended }]
            : prevBlock.pending,
        });
      } else {
        // Structural change — reset.
        result.push({
          key: prevBlock.key,
          type: p.type,
          level: p.level,
          committed: p.text,
          pending: [],
        });
      }
    } else {
      // Brand new last block (block count grew or type flipped).
      result.push({
        key: mint(),
        type: p.type,
        level: p.level,
        committed: "",
        pending: p.text ? [{ key: mint(), text: p.text }] : [],
      });
    }
  }

  return result;
}

/**
 * Group consecutive bulletListItem / numberedListItem blocks under a
 * synthetic list wrapper so the renderer can emit <ul>/<ol> around
 * their <li> children. Non-list blocks pass through untouched.
 *
 * Returns an array of { kind: 'block'|'list', ... } entries.
 */
export function groupListItems(blocks) {
  const out = [];
  let currentList = null;

  for (const b of blocks) {
    if (b.type === "bulletListItem" || b.type === "numberedListItem") {
      const listKind = b.type === "bulletListItem" ? "ul" : "ol";
      if (!currentList || currentList.listKind !== listKind) {
        currentList = { kind: "list", listKind, key: `list-${b.key}`, items: [] };
        out.push(currentList);
      }
      currentList.items.push(b);
    } else {
      currentList = null;
      out.push({ kind: "block", block: b });
    }
  }

  return out;
}
