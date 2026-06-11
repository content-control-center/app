import {
  createFileRoute,
  Outlet,
  useNavigate,
  useRouterState,
} from "@tanstack/react-router";
import { useMemo } from "react";
import { PageContainer } from "@/components/page-primitives/PageContainer.tsx";
import { PageLoader } from "@/components/page-primitives/PageLoader.tsx";
import { PageError } from "@/components/page-primitives/PageError.tsx";
import { PageHeader } from "@/components/page-primitives/PageHeader.tsx";
import { Button } from "@/components/ui/button.tsx";
import { GearSix, Layout, Plus } from "@phosphor-icons/react";
import { CampaignHeaderActions } from "@/components/campaigns/CampaignHeaderActions.tsx";
import { CampaignTabBar } from "@/components/campaigns/CampaignTabBar.tsx";
import { CampaignSettingsForm } from "@/components/forms/campaignSettingsForm";
import { CampaignContentUsageForm } from "@/components/forms/campaignContentUsageForm";
import { formatAnchor } from "@/components/campaigns/calendar/date";
import { useCampaign } from "@/hooks/useCampaigns.ts";
import { useAddPost } from "@/hooks/usePosts.ts";
import { useRightRailSection } from "@/hooks/useRightRailSection";
import { useRightRailPage } from "@/hooks/useRightRailPage";
import type { RightRailButton } from "@/stores/rightRailStore";

export const Route = createFileRoute("/_authenticated/campaigns/$campaignId")({
  component: CampaignLayout,
});

const LEFT_TABS = [
  { id: "calendar", label: "CALENDAR" },
  { id: "list", label: "LIST" },
];

const RIGHT_TABS = [{ id: "brief", label: "BRIEF" }];

function CampaignLayout() {
  const { campaignId } = Route.useParams();
  const { data: campaign, isLoading, isError } = useCampaign(campaignId);
  const navigate = useNavigate();
  const addPost = useAddPost(campaignId);

  // The active tab is derived from the URL rather than local state.
  const pathname = useRouterState({ select: (s) => s.location.pathname });
  const activeTab = pathname.includes("/brief")
    ? "brief"
    : pathname.includes("/list")
      ? "list"
      : "calendar";

  const railButtons = useMemo<RightRailButton[]>(
    () =>
      campaign
        ? [
            {
              id: "settings",
              icon: GearSix,
              ariaLabel: "Campaign settings",
              panel: ({ close }) => (
                <CampaignSettingsForm campaign={campaign} onClose={close} />
              ),
            },
            {
              id: "content-usage",
              icon: Layout,
              ariaLabel: "Content usage",
              panel: ({ close }) => (
                <CampaignContentUsageForm campaign={campaign} onClose={close} />
              ),
            },
          ]
        : [],
    [campaign],
  );
  useRightRailSection("campaign-detail", railButtons);
  useRightRailPage("campaign-detail", "settings");

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

  const handleTabSelect = (id: string) => {
    if (id === activeTab) return;
    if (id === "calendar") {
      // Always land on the current week when entering the calendar tab.
      navigate({
        to: "/campaigns/$campaignId/calendar/$anchor/$view",
        params: { campaignId, anchor: formatAnchor(new Date()), view: "week" },
      });
    } else {
      navigate({
        to: `/campaigns/$campaignId/${id}`,
        params: { campaignId },
      });
    }
  };

  const addPostButton = (
    <Button variant="default" size="default" onClick={addPost}>
      <Plus className="size-4" />
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
          onTabSelect={handleTabSelect}
          action={addPostButton}
        />
        <div className={"grid overflow-hidden h-full mt-1 px-3 lg:mt-2 lg:px-6"}>
          <Outlet />
        </div>
      </div>
    </PageContainer>
  );
}
