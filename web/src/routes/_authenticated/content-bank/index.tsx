import { createFileRoute, useNavigate } from "@tanstack/react-router";
import { PageContainer } from "@/components/page-primitives/PageContainer.tsx";
import { PageHeader } from "@/components/page-primitives/PageHeader.tsx";
import { PageLoader } from "@/components/page-primitives/PageLoader.tsx";
import { PageError } from "@/components/page-primitives/PageError.tsx";
import { Button } from "@/components/ui/button.tsx";
import { useAssets, useCreateAsset, useDeleteAsset } from "@/hooks/useContent.ts";
import { AssetsTable } from "@/components/tables/docsTable/index.tsx";
import {Icon} from "@/components/ui/icon.tsx";
import {PageGridEmptyState} from "@/components/page-primitives/PageGridEmptyState.tsx";
import { useRightRailPage } from "@/hooks/useRightRailPage";

export const Route = createFileRoute("/_authenticated/content-bank/")({
  component: ContentBank,
});

function ContentBank() {
  const { data: assets, isLoading, isError } = useAssets();
  const createAsset = useCreateAsset();
  const deleteAsset = useDeleteAsset();
  const navigate = useNavigate();
  const hasAssets = !!(assets && assets?.length > 0);
  useRightRailPage("content-bank", null);

  const handleCreate = () => {
    createAsset.mutate(
      { title: " ", content: " " },
      {
        onSuccess: (asset) => {
          navigate({ to: "/content-bank/$assetId", params: { assetId: asset.id } });
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
        <PageError header="Failed to load content assets" />
      </PageContainer>
    );
  }



  return (
    <PageContainer variant="fullFlex">
      <PageHeader
        title={"Content Bank"}
        className={'pt-6'}
        actions={
          <Button onClick={handleCreate} disabled={createAsset.isPending} size="lg">
            <Icon name={'plus'} className={'size-4'} /><span>ADD ASSET</span>
          </Button>
        }
      />
      <div className={'grid overflow-hidden h-full mt-1 px-3 lg:mt-2 lg:px-6'}>
        {hasAssets ? (
          <AssetsTable
            assets={assets ?? []}
            onDelete={(id) => deleteAsset.mutate(id)}
            emptyStateMessage="No content assets yet"
            emptyStateActionLabel="Add Asset"
            onEmptyStateAction={handleCreate}
          />
        ) : (
          <PageGridEmptyState
            title="No assets yet"
            subtitle="Create your first asset to start building your content bank"
            actions={
              <Button onClick={handleCreate} disabled={createAsset.isPending} variant="defaultInverted">
                <Icon name={'plus'} className={'size-4 stroke-[2px]'} />
                <span>ADD ASSET</span>
              </Button>
            }
          />
        )}


      </div>
    </PageContainer>
  );
}
