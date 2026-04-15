import { createFileRoute } from "@tanstack/react-router";
import { useCallback, useState } from "react";
import { PageContainer } from "@/components/page-primitives/PageContainer.tsx";
import { PageLoader } from "@/components/page-primitives/PageLoader.tsx";
import { PageError } from "@/components/page-primitives/PageError.tsx";
import { PageHeader } from "@/components/page-primitives/PageHeader.tsx";
import { RightRail } from "@/components/page-primitives/RightRail.tsx";
import { ScrollArea } from "@/components/ui/scroll-area.tsx";
import { CampaignTabBar } from "@/components/campaigns/CampaignTabBar.tsx";
import { BriefForm } from "@/components/campaigns/BriefForm.tsx";
import { CampaignHeaderActions } from "@/components/campaigns/CampaignHeaderActions.tsx";
import { useCampaign } from "@/hooks/useCampaigns.ts";

export const Route = createFileRoute("/_authenticated/campaigns/$campaignId")({
  component: CampaignPage,
});

const TABS = [
  { id: "brief", label: "BRIEF" },
  { id: "schedule", label: "SCHEDULE" },
];

function CampaignPage() {
  const { campaignId } = Route.useParams();
  const { data: campaign, isLoading, isError } = useCampaign(campaignId);
  const [activeTab, setActiveTab] = useState<string>("brief");
  const [briefTitle, setBriefTitle] = useState<string | null>(null);
  const [briefDirty, setBriefDirty] = useState(false);

  const handleTitleChange = useCallback((t: string) => setBriefTitle(t), []);
  const handleDirtyChange = useCallback((d: boolean) => setBriefDirty(d), []);

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

  const displayName = (briefTitle ?? campaign.name).trim();
  const title = displayName === "" ? "Untitled campaign" : displayName;

  return (
    <PageContainer variant={"fullFlex"}>
      <div className={"flex-1 min-h-0 flex flex-row"}>
        <div className={"flex-1 min-h-0 flex flex-col"}>
          <PageHeader
            title={title}
            overlay={"campaign-selector"}
            className={"pt-6"}
            unsaved={briefDirty}
            actions={<CampaignHeaderActions campaign={campaign} />}
          />
          <CampaignTabBar
            activeTab={activeTab}
            tabs={TABS}
            onTabSelect={setActiveTab}
          />
          <ScrollArea className={"flex-1 min-h-0 mx-6 mt-2"} type={"scroll"} scrollHideDelay={350}>
              {activeTab === "brief" && (
                  <BriefForm
                    campaign={campaign}
                    onTitleChange={handleTitleChange}
                    onDirtyChange={handleDirtyChange}
                  />
              )}
              {activeTab === "schedule" && <div>Schedule coming soon</div>}
          </ScrollArea>
        </div>
        <RightRail
          buttons={[
            {
              id: "settings",
              icon: "settings",
              ariaLabel: "Settings",
              panel: <div className="text-sm">Settings panel</div>,
            },
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
