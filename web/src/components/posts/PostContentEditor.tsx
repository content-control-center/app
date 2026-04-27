import { useCallback, useEffect, useRef } from 'react'
import { useCreateBlockNote } from '@blocknote/react'
import { BlockNoteView } from '@blocknote/mantine'
import type { Block } from '@blocknote/core'
import '@blocknote/mantine/style.css'
import '@/blocknote-theme.css'

type PostContentEditorProps = {
  initialContent: string
  onContentChange: (content: string) => void
}

const DEFAULT_CONTENT: Block[] = [
  { type: 'paragraph', props: {}, content: [] } as unknown as Block,
]

export function PostContentEditor({
  initialContent,
  onContentChange,
}: PostContentEditorProps) {
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const readyRef = useRef(false)
  const editor = useCreateBlockNote({ initialContent: DEFAULT_CONTENT })

  useEffect(() => {
    const blocks = editor.tryParseMarkdownToBlocks(initialContent ?? '')
    const next = blocks.length > 0 ? blocks : DEFAULT_CONTENT
    editor.replaceBlocks(editor.document, next)
    readyRef.current = true
  }, [editor])

  const handleChange = useCallback(() => {
    if (!readyRef.current) return
    if (timerRef.current) clearTimeout(timerRef.current)
    timerRef.current = setTimeout(() => {
      onContentChange(editor.blocksToMarkdownLossy())
    }, 500)
  }, [editor, onContentChange])

  useEffect(() => {
    return () => {
      if (timerRef.current) clearTimeout(timerRef.current)
    }
  }, [])

  return <BlockNoteView editor={editor} onChange={handleChange} theme="light" />
}
