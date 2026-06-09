import { useCallback, useEffect, useRef } from 'react'
import { useCreateBlockNote } from '@blocknote/react'
import { BlockNoteView } from '@blocknote/mantine'
import { BlockNoteSchema, defaultBlockSpecs } from '@blocknote/core'
import '@blocknote/mantine/style.css'
import '@/blocknote-theme.css'
import { uploadImage } from '@/services/api/images'

// Image block is intentionally disabled here — posts are per-platform and
// the image affordance will be re-enabled conditionally per post type.
const { image: _image, ...postBlockSpecs } = defaultBlockSpecs
const postSchema = BlockNoteSchema.create({ blockSpecs: postBlockSpecs })

type PostContentEditorProps = {
  initialContent: string
  onContentChange: (content: string) => void
}

const DEFAULT_CONTENT = [{ type: 'paragraph' as const }]

export function PostContentEditor({
  initialContent,
  onContentChange,
}: PostContentEditorProps) {
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const readyRef = useRef(false)
  const editor = useCreateBlockNote({
    schema: postSchema,
    initialContent: DEFAULT_CONTENT,
    uploadFile: uploadImage,
  })

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
