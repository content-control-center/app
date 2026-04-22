import { useCallback, useEffect, useRef } from 'react'
import { useCreateBlockNote } from '@blocknote/react'
import { BlockNoteView } from '@blocknote/mantine'
import type { Block } from '@blocknote/core'
import '@blocknote/mantine/style.css'
import '@/blocknote-theme.css'

type PostContentEditorProps = {
  initialContent: string
  onContentChange: (content: string) => void
  onDirty?: () => void
}

const DEFAULT_CONTENT: Block[] = [
  { type: 'paragraph', props: {}, content: [] } as unknown as Block,
]

function parseBlocks(raw: string): Block[] {
  if (!raw || raw.trim() === '') return DEFAULT_CONTENT
  try {
    const parsed = JSON.parse(raw)
    if (Array.isArray(parsed) && parsed.length > 0) return parsed as Block[]
  } catch {
    return [
      { type: 'paragraph', props: {}, content: raw } as unknown as Block,
    ]
  }
  return DEFAULT_CONTENT
}

export function PostContentEditor({
  initialContent,
  onContentChange,
  onDirty,
}: PostContentEditorProps) {
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const editor = useCreateBlockNote({ initialContent: parseBlocks(initialContent) })

  const handleChange = useCallback(() => {
    onDirty?.()
    if (timerRef.current) clearTimeout(timerRef.current)
    timerRef.current = setTimeout(() => {
      onContentChange(JSON.stringify(editor.document))
    }, 500)
  }, [editor, onContentChange, onDirty])

  useEffect(() => {
    return () => {
      if (timerRef.current) clearTimeout(timerRef.current)
    }
  }, [])

  return <BlockNoteView editor={editor} onChange={handleChange} theme="light" />
}
