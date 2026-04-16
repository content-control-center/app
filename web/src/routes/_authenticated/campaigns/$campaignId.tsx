import { createFileRoute } from "@tanstack/react-router";
import { useState } from "react";
import { PageContainer } from "@/components/page-primitives/PageContainer.tsx";
import { PageLoader } from "@/components/page-primitives/PageLoader.tsx";
import { PageError } from "@/components/page-primitives/PageError.tsx";
import { PageHeader } from "@/components/page-primitives/PageHeader.tsx";
import { PageGridEmptyState } from "@/components/page-primitives/PageGridEmptyState.tsx";
import { RightRail } from "@/components/page-primitives/RightRail.tsx";
import { Button } from "@/components/ui/button.tsx";
import { Icon } from "@/components/ui/icon.tsx";
import { CampaignSettingsButton } from "@/components/ui/campaign-settings-button.tsx";
import { CampaignHeaderActions } from "@/components/campaigns/CampaignHeaderActions.tsx";
import { CampaignTabBar } from "@/components/campaigns/CampaignTabBar.tsx";
import { useCampaign } from "@/hooks/useCampaigns.ts";
import { useOverlayStore } from "@/stores/overlayStore.ts";

export const Route = createFileRoute("/_authenticated/campaigns/$campaignId")({
  component: CampaignPage,
});

const TABS = [
  { id: "calendar", label: "CALENDAR" },
  { id: "list", label: "LIST" },
];

function CampaignPage() {
  const { campaignId } = Route.useParams();
  const { data: campaign, isLoading, isError } = useCampaign(campaignId);
  const [activeTab, setActiveTab] = useState<string>("calendar");
  const openOverlay = useOverlayStore((s) => s.open);

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

  const handleOpenSettings = () => {
    openOverlay("campaign-settings", { campaignId: campaign.id });
  };

  const handleAddPost = () => {
    // TODO: wire to post creation flow
  };

  const addPostButton = (
    <Button variant="default" size="sm" onClick={handleAddPost}>
      <Icon name="plus" className="size-4 stroke-[2px]" />
      <span>ADD POST</span>
    </Button>
  );

  return (
    <PageContainer variant={"fullFlex"}>
      <div className={"flex-1 min-h-0 flex flex-row"}>
        <div className={"flex-1 min-h-0 flex flex-col"}>
          <PageHeader
            title={title}
            overlay={"campaign-selector"}
            className={"pt-6"}
            actions={
              <div className="flex items-center gap-3">
                <CampaignHeaderActions campaign={campaign} />
                <CampaignSettingsButton
                  campaign={campaign}
                  onOpen={handleOpenSettings}
                />
              </div>
            }
          />
          <CampaignTabBar
            activeTab={activeTab}
            tabs={TABS}
            onTabSelect={setActiveTab}
            action={addPostButton}
          />
          <div className={"grid overflow-hidden h-full mt-1 px-3 lg:mt-2 lg:px-6"}>
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
          </div>
        </div>
        <RightRail
          buttons={[
            {
              id: "ai",
              icon: "strategy",
              ariaLabel: "AI assistant",
              panel: <div className="text-sm">AI assistant panel</div>,
            },
            {
              id: "stats",
              icon: "trend_up",
              ariaLabel: "Stats",
              panel: <div className="text-sm">Stats panel</div>,
            },
          ]}
        />
      </div>
    </PageContainer>
  );
}
