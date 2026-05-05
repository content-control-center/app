import { createFileRoute } from '@tanstack/react-router'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { PageContainer } from '@/components/page-primitives/PageContainer'
import { PageLoader } from '@/components/page-primitives/PageLoader'
import { PageError } from '@/components/page-primitives/PageError'
import { EditPageHeader } from '@/components/page-primitives/EditPageHeader'
import { ScrollArea } from '@/components/ui/scroll-area'
import { useCampaign } from '@/hooks/useCampaigns'
import { useRightRailSection } from '@/hooks/useRightRailSection'
import { useRightRailPage } from '@/hooks/useRightRailPage'
import type { RightRailButton } from '@/stores/rightRailStore'
import { PostSettingsForm } from '@/components/forms/postSettingsForm'
import { PostContentUsageForm } from '@/components/forms/postContentUsageForm'
import { PostContentEditor } from '@/components/posts/PostContentEditor'
import { PostStatusHeaderActions } from '@/components/posts/PostStatusHeaderActions'
import { usePost, type TransitionStatusResult } from '@/hooks/usePost'
import type { Post, PostStatus } from '@/types/posts'

export const Route = createFileRoute(
  '/_authenticated/campaigns/$campaignId_/posts/$postId',
)({
  component: PostPage,
})

function PostPage() {
  const { campaignId, postId } = Route.useParams()
  const { doc, changeDoc, transitionStatus, loading, error } = usePost(postId)
  const { data: campaign } = useCampaign(campaignId)

  const railButtons = useMemo<RightRailButton[]>(
    () =>
      doc
        ? [
            {
              id: 'settings',
              icon: 'settings',
              ariaLabel: 'Post settings',
              panel: ({ close }) => (
                <PostSettingsForm doc={doc} changeDoc={changeDoc} onClose={close} />
              ),
            },
            {
              id: 'content-usage',
              icon: 'layout',
              ariaLabel: 'Content pieces',
              panel: ({ close }) => (
                <PostContentUsageForm doc={doc} changeDoc={changeDoc} onClose={close} />
              ),
            },
          ]
        : [],
    [doc, changeDoc],
  )
  useRightRailSection('post-detail', railButtons)
  useRightRailPage('post-detail', 'settings')

  if (loading) {
    return (
      <PageContainer>
        <PageLoader />
      </PageContainer>
    )
  }

  if (error || !doc) {
    return (
      <PageContainer>
        <PageError header="Post not found" />
      </PageContainer>
    )
  }

  return (
    <PostEditorSurface
      doc={doc}
      changeDoc={changeDoc}
      transitionStatus={transitionStatus}
      campaignId={campaignId}
      campaignName={campaign?.name?.trim() || 'Campaign'}
    />
  )
}

type PostEditorSurfaceProps = {
  doc: Post
  changeDoc: (fn: (p: Post) => void) => void
  transitionStatus: (next: PostStatus) => Promise<TransitionStatusResult>
  campaignId: string
  campaignName: string
}

function PostEditorSurface({
  doc,
  changeDoc,
  transitionStatus,
  campaignId,
  campaignName,
}: PostEditorSurfaceProps) {
  const [titleDraft, setTitleDraft] = useState(doc.title)
  const titleRef = useRef<HTMLTextAreaElement | null>(null)

  const autosizeTitle = useCallback(() => {
    const el = titleRef.current
    if (!el) return
    el.style.height = 'auto'
    el.style.height = `${el.scrollHeight}px`
  }, [])

  useEffect(() => {
    autosizeTitle()
  }, [titleDraft, autosizeTitle])

  const handleTitleChange = useCallback(
    (next: string) => {
      setTitleDraft(next)
      changeDoc((d) => {
        d.title = next
      })
    },
    [changeDoc],
  )

  const handleContentChange = useCallback(
    (next: string) => {
      changeDoc((d) => {
        d.content = next
      })
    },
    [changeDoc],
  )

  const displayTitle = titleDraft.trim() === '' ? 'Untitled' : titleDraft

  return (
    <PageContainer variant="fullFlex">
      <ScrollArea className="flex-1 min-h-0 lg:px-6" type="scroll" scrollHideDelay={350}>
        <EditPageHeader
          title={displayTitle}
          breadcrumbs={[{ label: campaignName, to: `/campaigns/${campaignId}` }]}
          actions={
            <PostStatusHeaderActions post={doc} transitionStatus={transitionStatus} />
          }
        />
        <div className="flex flex-col items-center gap-0 relative z-0">
          <div className="w-[740px] bg-white px-16 py-8 mt-2 mb-8">
            <div className="flex flex-col">
              <textarea
                ref={titleRef}
                value={titleDraft}
                onChange={(e) => {
                  const next = e.target.value.replace(/\n/g, '')
                  handleTitleChange(next)
                }}
                placeholder="Title"
                rows={1}
                className="resize-none overflow-hidden bg-transparent border-0 outline-none w-full text-4xl font-bold tracking-tight placeholder:text-tertiary-foreground mb-4"
              />
              <PostContentEditor
                initialContent={doc.content}
                onContentChange={handleContentChange}
              />
            </div>
          </div>
        </div>
      </ScrollArea>
    </PageContainer>
  )
}
