import { createFileRoute, useNavigate } from "@tanstack/react-router";
import { Button } from "@/components/ui/button.tsx";
import { PageContainer } from "@/components/page-primitives/PageContainer.tsx";
import { PageLoader } from "@/components/page-primitives/PageLoader.tsx";
import { PageError } from "@/components/page-primitives/PageError.tsx";
import { PageHeader } from "@/components/page-primitives/PageHeader.tsx";
import { PageGridEmptyState } from "@/components/page-primitives/PageGridEmptyState.tsx";
import { Plus } from "@phosphor-icons/react";
import { CampaignCard } from "@/components/campaigns/CampaignCard.tsx";
import {
  useCampaigns,
  useCreateCampaign,
  useCampaignTypes,
} from "@/hooks/useCampaigns.ts";
import { useRightRailPage } from "@/hooks/useRightRailPage";

export const Route = createFileRoute("/_authenticated/campaigns/")({
  component: Campaigns,
});

function Campaigns() {
  const { data: campaigns, isLoading, isError } = useCampaigns();
  const {
    data: campaignTypes,
    isLoading: isTypesLoading,
    error: typesError,
  } = useCampaignTypes();
  const createCampaign = useCreateCampaign();
  const navigate = useNavigate();
  useRightRailPage("campaigns-list", null);

  const hasCampaigns = !!(campaigns && campaigns.length > 0);
  const firstType = campaignTypes?.[0];
  const canCreate = !!firstType && !createCampaign.isPending && !isTypesLoading;

  console.log("[campaigns] render", {
    campaignTypes,
    isTypesLoading,
    typesError,
    createPending: createCampaign.isPending,
    canCreate,
  });

  const handleCreate = () => {
    if (!firstType) return;
    createCampaign.mutate(
      { name: " ", campaign_type_id: firstType.id },
      {
        onSuccess: (campaign) => {
          navigate({
            to: "/campaigns/$campaignId",
            params: { campaignId: campaign.id },
          });
        },
      },
    );
  };

  if (isLoading) {
    return (
      <PageContainer>
        <PageLoader />
      </PageContainer>
    );
  }

  if (isError) {
    return (
      <PageContainer>
        <PageError header="Failed to load campaigns" />
      </PageContainer>
    );
  }

  return (
    <PageContainer variant="fullFlex">
      <PageHeader
        title={"Campaigns"}
        className={'pt-6'}
        actions={
          <Button onClick={handleCreate} disabled={!canCreate} size="lg">
            <Plus className={'size-4'} />
            <span>ADD CAMPAIGN</span>
          </Button>
        }
      />
      <div className={'grid overflow-hidden h-full mt-1 px-3 lg:mt-2 lg:px-6'}>
        {hasCampaigns ? (
          <ul className="flex flex-col gap-6 overflow-auto py-2">
            {campaigns!.map((campaign) => (
              <li key={campaign.id}>
                <CampaignCard
                  campaign={campaign}
                  onClick={() =>
                    navigate({
                      to: "/campaigns/$campaignId",
                      params: { campaignId: campaign.id },
                    })
                  }
                />
              </li>
            ))}
          </ul>
        ) : (
          <PageGridEmptyState
            title="No campaigns yet"
            subtitle="Create your first campaign to get started"
            actions={
              <Button onClick={handleCreate} disabled={!canCreate} variant="defaultInverted">
                <Plus className={'size-4'} />
                <span>ADD CAMPAIGN</span>
              </Button>
            }
          />
        )}
      </div>
    </PageContainer>
  );
}
