import { createFileRoute } from '@tanstack/react-router'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { PageContainer } from '@/components/page-primitives/PageContainer'
import { PageLoader } from '@/components/page-primitives/PageLoader'
import { PageError } from '@/components/page-primitives/PageError'
import { EditPageHeader } from '@/components/page-primitives/EditPageHeader'
import { ScrollArea } from '@/components/ui/scroll-area'
import { usePost, useUpdatePost } from '@/hooks/usePosts'
import { useCampaign } from '@/hooks/useCampaigns'
import { useRightRailSection } from '@/hooks/useRightRailSection'
import type { RightRailButton } from '@/stores/rightRailStore'
import { AIAssistantPanel, StatsPanel } from '@/components/rail-panels/ComingSoonPanel'
import { PostSettingsForm } from '@/components/forms/postSettingsForm'
import { PostContentUsageForm } from '@/components/forms/postContentUsageForm'
import { PostContentEditor } from '@/components/posts/PostContentEditor'
import { PostStatusBadge } from '@/components/posts/PostStatusBadge'

export const Route = createFileRoute(
  '/_authenticated/campaigns/$campaignId_/posts/$postId',
)({
  component: PostPage,
})

function PostPage() {
  const { campaignId, postId } = Route.useParams()
  const { data: post, isLoading, isError } = usePost(postId)
  const { data: campaign } = useCampaign(campaignId)
  const updatePost = useUpdatePost(postId, campaignId)

  const [title, setTitle] = useState<string | null>(null)
  const [content, setContent] = useState<string | null>(null)

  const editVersionRef = useRef(0)
  const [editVersion, setEditVersion] = useState(0)
  const [savedVersion, setSavedVersion] = useState(0)
  const isDirty = editVersion !== savedVersion

  const titleTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const titleRef = useRef<HTMLTextAreaElement | null>(null)

  const currentTitle = title ?? post?.title ?? ''
  const currentContent = content ?? post?.content ?? ''

  const markDirty = useCallback(() => {
    editVersionRef.current += 1
    setEditVersion(editVersionRef.current)
  }, [])

  const save = useCallback(
    (nextTitle: string, nextContent: string) => {
      if (!post) return
      const v = editVersionRef.current
      updatePost.mutate(
        {
          campaign_id: post.campaign_id,
          platform_id: post.platform_id,
          platform_post_type: post.platform_post_type,
          title: nextTitle,
          content: nextContent,
          status: post.status,
          cta_type: post.cta_type,
          cta_url: post.cta_url,
          target_audience_notes: post.target_audience_notes,
          used_asset_ids: post.used_asset_ids,
          campaign_type_phase_id: post.campaign_type_phase_id,
        },
        { onSuccess: () => setSavedVersion(v) },
      )
    },
    [post, updatePost],
  )

  const handleTitleChange = useCallback(
    (nextTitle: string) => {
      setTitle(nextTitle)
      markDirty()
      if (titleTimerRef.current) clearTimeout(titleTimerRef.current)
      titleTimerRef.current = setTimeout(() => {
        save(nextTitle, content ?? post?.content ?? '')
      }, 500)
    },
    [markDirty, save, content, post],
  )

  const handleContentChange = useCallback(
    (nextContent: string) => {
      setContent(nextContent)
      save(title ?? post?.title ?? '', nextContent)
    },
    [save, title, post],
  )

  const autosizeTitle = useCallback(() => {
    const el = titleRef.current
    if (!el) return
    el.style.height = 'auto'
    el.style.height = `${el.scrollHeight}px`
  }, [])

  useEffect(() => {
    autosizeTitle()
  }, [currentTitle, autosizeTitle])

  useEffect(() => {
    return () => {
      if (titleTimerRef.current) clearTimeout(titleTimerRef.current)
    }
  }, [])

  const railButtons = useMemo<RightRailButton[]>(
    () =>
      post
        ? [
            {
              id: 'settings',
              icon: 'settings',
              ariaLabel: 'Post settings',
              panel: ({ close }) => <PostSettingsForm post={post} onClose={close} />,
            },
            {
              id: 'content-usage',
              icon: 'layout',
              ariaLabel: 'Content pieces',
              panel: ({ close }) => (
                <PostContentUsageForm post={post} onClose={close} />
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
    [post],
  )
  useRightRailSection('post-detail', railButtons)

  if (isLoading) {
    return (
      <PageContainer>
        <PageLoader />
      </PageContainer>
    )
  }

  if (isError || !post) {
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
            breadcrumbs={[
              { label: campaignName, to: `/campaigns/${campaignId}` },
            ]}
            unsaved={isDirty}
            breadcrumbTrailing={<PostStatusBadge status={post.status} />}
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
                  initialContent={currentContent}
                  onContentChange={handleContentChange}
                  onDirty={markDirty}
                />
              </div>
            </div>
          </div>
      </ScrollArea>
    </PageContainer>
  )
}
