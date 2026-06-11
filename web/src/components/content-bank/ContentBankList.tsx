import { useNavigate } from "@tanstack/react-router";
import { PageLoader } from "@/components/page-primitives/PageLoader.tsx";
import { PageError } from "@/components/page-primitives/PageError.tsx";
import { PageGridEmptyState } from "@/components/page-primitives/PageGridEmptyState.tsx";
import { Button } from "@/components/ui/button.tsx";
import { Plus } from "@phosphor-icons/react";
import { AssetsTable } from "@/components/tables/docsTable/index.tsx";
import { useAssets, useCreateAsset, useDeleteAsset } from "@/hooks/useContent.ts";
import { assetCategory, type ContentBankTab } from "@/lib/assetCategory";

const EMPTY_MESSAGE: Record<ContentBankTab, string> = {
  all: "No content assets yet",
  text: "No text assets yet",
  imagery: "No imagery assets yet",
  files: "No files yet",
};

/** Renders the Content Bank asset table for a single tab, filtered by category. */
export function ContentBankList({ category }: { category: ContentBankTab }) {
  const { data: assets, isLoading, isError } = useAssets();
  const createAsset = useCreateAsset();
  const deleteAsset = useDeleteAsset();
  const navigate = useNavigate();

  const handleCreate = () => {
    createAsset.mutate(
      { title: " ", content: " " },
      {
        onSuccess: (asset) => {
          navigate({
            to: "/content-bank/$assetId",
            params: { assetId: asset.id },
          });
        },
      },
    );
  };

  if (isLoading) return <PageLoader />;
  if (isError) return <PageError header="Failed to load content assets" />;

  const all = assets ?? [];

  // An empty bank gets the inviting create prompt regardless of the active tab.
  if (all.length === 0) {
    return (
      <PageGridEmptyState
        title="No assets yet"
        subtitle="Create your first asset to start building your content bank"
        actions={
          <Button
            onClick={handleCreate}
            disabled={createAsset.isPending}
            variant="defaultInverted"
          >
            <Plus className="size-4" />
            <span>ADD ASSET</span>
          </Button>
        }
      />
    );
  }

  const filtered =
    category === "all"
      ? all
      : all.filter((asset) => assetCategory(asset) === category);

  return (
    <AssetsTable
      assets={filtered}
      onDelete={(id) => deleteAsset.mutate(id)}
      emptyStateMessage={EMPTY_MESSAGE[category]}
    />
  );
}
