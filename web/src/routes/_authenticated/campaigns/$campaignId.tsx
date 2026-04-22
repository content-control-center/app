import { createFileRoute, useNavigate } from "@tanstack/react-router";
import { useMemo, useState } from "react";
import { PageContainer } from "@/components/page-primitives/PageContainer.tsx";
import { PageLoader } from "@/components/page-primitives/PageLoader.tsx";
import { PageError } from "@/components/page-primitives/PageError.tsx";
import { PageHeader } from "@/components/page-primitives/PageHeader.tsx";
import { PageGridEmptyState } from "@/components/page-primitives/PageGridEmptyState.tsx";
import { Button } from "@/components/ui/button.tsx";
import { Icon } from "@/components/ui/icon.tsx";
import { CampaignHeaderActions } from "@/components/campaigns/CampaignHeaderActions.tsx";
import { CampaignTabBar } from "@/components/campaigns/CampaignTabBar.tsx";
import { PostsTable } from "@/components/tables/postsTable";
import { WeeklyCalendar } from "@/components/campaigns/calendar/WeeklyCalendar";
import { CampaignBriefForm } from "@/components/forms/campaignBriefForm";
import { CampaignSettingsForm } from "@/components/forms/campaignSettingsForm";
import { CampaignContentUsageForm } from "@/components/forms/campaignContentUsageForm";
import { AIAssistantPanel, StatsPanel } from "@/components/rail-panels/ComingSoonPanel";
import { useCampaign } from "@/hooks/useCampaigns.ts";
import { useCampaignPosts, useCreatePost, useDeletePost } from "@/hooks/usePosts.ts";
import { usePlatforms } from "@/hooks/usePlatforms.ts";
import { useRightRailSection } from "@/hooks/useRightRailSection";
import type { RightRailButton } from "@/stores/rightRailStore";

export const Route = createFileRoute("/_authenticated/campaigns/$campaignId")({
  component: CampaignPage,
});

const LEFT_TABS = [
  { id: "calendar", label: "CALENDAR" },
  { id: "list", label: "LIST" },
];

const RIGHT_TABS = [{ id: "brief", label: "BRIEF" }];

function CampaignPage() {
  const { campaignId } = Route.useParams();
  const { data: campaign, isLoading, isError } = useCampaign(campaignId);
  const { data: posts } = useCampaignPosts(campaignId);
  const createPost = useCreatePost(campaignId);
  const deletePost = useDeletePost(campaignId);
  const { data: platforms } = usePlatforms();
  const navigate = useNavigate();
  const [activeTab, setActiveTab] = useState<string>("calendar");
  const hasPosts = !!(posts && posts.length > 0);

  const railButtons = useMemo<RightRailButton[]>(
    () =>
      campaign
        ? [
            {
              id: "settings",
              icon: "settings",
              ariaLabel: "Campaign settings",
              panel: ({ close }) => (
                <CampaignSettingsForm campaign={campaign} onClose={close} />
              ),
            },
            {
              id: "content-usage",
              icon: "layout",
              ariaLabel: "Content usage",
              panel: ({ close }) => (
                <CampaignContentUsageForm campaign={campaign} onClose={close} />
              ),
            },
            {
              id: "ai",
              icon: "strategy",
              ariaLabel: "AI assistant",
              panel: ({ close }) => <AIAssistantPanel onClose={close} />,
            },
            {
              id: "stats",
              icon: "trend_up",
              ariaLabel: "Stats",
              panel: ({ close }) => <StatsPanel onClose={close} />,
            },
          ]
        : [],
    [campaign],
  );
  useRightRailSection("campaign-detail", railButtons);

  if (isLoading) {
    return (
      <PageContainer>
        <PageLoader />
      </PageContainer>
    );
  }

  if (isError || !campaign) {
    return (
      <PageContainer>
        <PageError header="Campaign not found" />
      </PageContainer>
    );
  }

  const displayName = campaign.name.trim();
  const title = displayName === "" ? "Untitled campaign" : displayName;

  const handleAddPost = () => {
    const firstPlatform = platforms?.[0];
    if (!firstPlatform) return;
    const firstPostType = Object.keys(firstPlatform.post_types ?? {})[0] ?? 'post';
    createPost.mutate(
      {
        campaign_id: campaignId,
        platform_id: firstPlatform.id,
        platform_post_type: firstPostType,
      },
      {
        onSuccess: (post) => {
          navigate({
            to: '/campaigns/$campaignId/posts/$postId',
            params: { campaignId, postId: post.id },
          });
        },
      },
    );
  };

  const addPostButton = (
    <Button variant="default" size="default" onClick={handleAddPost}>
      <Icon name="plus" className="size-4 stroke-[2px]" />
      <span>ADD POST</span>
    </Button>
  );

  return (
    <PageContainer variant={"fullFlex"}>
      <div className={"flex-1 min-h-0 flex flex-col"}>
        <PageHeader
          title={title}
          overlay={"campaign-selector"}
          className={"pt-6"}
          actions={
            <div className="flex items-center gap-3">
              <CampaignHeaderActions campaign={campaign} />
            </div>
          }
        />
        <CampaignTabBar
          activeTab={activeTab}
          tabs={LEFT_TABS}
          rightTabs={RIGHT_TABS}
          onTabSelect={setActiveTab}
          action={addPostButton}
        />
        <div className={"grid overflow-hidden h-full mt-1 px-3 lg:mt-2 lg:px-6"}>
          {activeTab === "calendar" ? (
            hasPosts ? (
              <WeeklyCalendar posts={posts ?? []} />
            ) : (
              <PageGridEmptyState
                title="No posts yet"
                subtitle="Add your first post to start building this campaign"
                actions={
                  <Button variant="defaultInverted" onClick={handleAddPost}>
                    <Icon name="plus" className="size-4 stroke-[2px]" />
                    <span>ADD POST</span>
                  </Button>
                }
              />
            )
          ) : activeTab === "list" ? (
            hasPosts ? (
              <PostsTable
                posts={posts ?? []}
                campaignId={campaignId}
                onDelete={(id) => deletePost.mutate(id)}
                emptyStateMessage="No posts yet"
                emptyStateActionLabel="Add Post"
                onEmptyStateAction={handleAddPost}
              />
            ) : (
              <PageGridEmptyState
                title="No posts yet"
                subtitle="Add your first post to start building this campaign"
                actions={
                  <Button variant="defaultInverted" onClick={handleAddPost}>
                    <Icon name="plus" className="size-4 stroke-[2px]" />
                    <span>ADD POST</span>
                  </Button>
                }
              />
            )
          ) : activeTab === "brief" ? (
            <div className="overflow-y-auto pb-6">
              <CampaignBriefForm campaign={campaign} />
            </div>
          ) : null}
        </div>
      </div>
    </PageContainer>
  );
}
