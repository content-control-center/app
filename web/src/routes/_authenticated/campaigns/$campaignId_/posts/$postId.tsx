import { createFileRoute } from '@tanstack/react-router'
import { useCallback, useEffect, useMemo, useRef } from 'react'
import { PageContainer } from '@/components/page-primitives/PageContainer'
import { PageLoader } from '@/components/page-primitives/PageLoader'
import { PageError } from '@/components/page-primitives/PageError'
import { EditPageHeader } from '@/components/page-primitives/EditPageHeader'
import { ScrollArea } from '@/components/ui/scroll-area'
import { useCampaign } from '@/hooks/useCampaigns'
import { useRightRailSection } from '@/hooks/useRightRailSection'
import type { RightRailButton } from '@/stores/rightRailStore'
import { AIAssistantPanel, StatsPanel } from '@/components/rail-panels/ComingSoonPanel'
import { PostSettingsForm } from '@/components/forms/postSettingsForm'
import { PostContentUsageForm } from '@/components/forms/postContentUsageForm'
import { PostContentEditor } from '@/components/posts/PostContentEditor'
import { PostStatusBadge } from '@/components/posts/PostStatusBadge'
import { usePost } from '@/hooks/usePost'

export const Route = createFileRoute(
  '/_authenticated/campaigns/$campaignId_/posts/$postId',
)({
  component: PostPage,
})

function PostPage() {
  const { campaignId, postId } = Route.useParams()
  const { doc, changeDoc, loading, error } = usePost(postId)
  const { data: campaign } = useCampaign(campaignId)

  const titleRef = useRef<HTMLTextAreaElement | null>(null)
  const autosizeTitle = useCallback(() => {
    const el = titleRef.current
    if (!el) return
    el.style.height = 'auto'
    el.style.height = `${el.scrollHeight}px`
  }, [])

  const currentTitle = doc?.title ?? ''

  useEffect(() => {
    autosizeTitle()
  }, [currentTitle, autosizeTitle])

  const handleTitleChange = useCallback(
    (next: string) => {
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
            {
              id: 'ai',
              icon: 'strategy',
              ariaLabel: 'AI assistant',
              panel: ({ close }) => <AIAssistantPanel onClose={close} />,
            },
            {
              id: 'stats',
              icon: 'trend_up',
              ariaLabel: 'Stats',
              panel: ({ close }) => <StatsPanel onClose={close} />,
            },
          ]
        : [],
    [doc, changeDoc],
  )
  useRightRailSection('post-detail', railButtons)

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

  const displayTitle = currentTitle.trim() === '' ? 'Untitled' : currentTitle
  const campaignName = campaign?.name?.trim() || 'Campaign'

  return (
    <PageContainer variant="fullFlex">
      <ScrollArea className="flex-1 min-h-0 lg:px-6" type="scroll" scrollHideDelay={350}>
        <EditPageHeader
          title={displayTitle}
          breadcrumbs={[{ label: campaignName, to: `/campaigns/${campaignId}` }]}
          breadcrumbTrailing={<PostStatusBadge status={doc.status} />}
        />
        <div className="flex flex-col items-center gap-0 relative z-0">
          <div className="w-[740px] bg-white px-16 py-8 mt-2 mb-8">
            <div className="flex flex-col">
              <textarea
                ref={titleRef}
                value={currentTitle}
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
