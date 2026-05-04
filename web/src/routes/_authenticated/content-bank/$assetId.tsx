import { createFileRoute } from "@tanstack/react-router";
import { PageContainer } from "@/components/page-primitives/PageContainer.tsx";
import { PageLoader } from "@/components/page-primitives/PageLoader.tsx";
import { PageError } from "@/components/page-primitives/PageError.tsx";
import { useAsset, useUpdateAsset } from "@/hooks/useContent.ts";
import { useCallback, useMemo, useRef, useState } from "react";
import { AssetEditor } from "@/components/content-bank/AssetEditor.tsx";
import {EditPageHeader} from "@/components/page-primitives/EditPageHeader.tsx";
import { ScrollArea } from "@/components/ui/scroll-area.tsx";
import { useRightRailSection } from "@/hooks/useRightRailSection";
import { useRightRailPage } from "@/hooks/useRightRailPage";
import { ComingSoonPanel } from "@/components/rail-panels/ComingSoonPanel";
import type { RightRailButton } from "@/stores/rightRailStore";

export const Route = createFileRoute("/_authenticated/content-bank/$assetId")({
  component: AssetPage,
});

function AssetPage() {
  const { assetId } = Route.useParams();
  const { data: asset, isLoading, isError } = useAsset(assetId);
  const updateAsset = useUpdateAsset();
  const [title, setTitle] = useState<string | null>(null);
  const editVersionRef = useRef(0);
  const [editVersion, setEditVersion] = useState(0);
  const [savedVersion, setSavedVersion] = useState(0);
  const isDirty = editVersion !== savedVersion;

  const markDirty = useCallback(() => {
    editVersionRef.current += 1;
    setEditVersion(editVersionRef.current);
  }, []);

  const handleTitleChange = useCallback(
    async (nextTitle: string) => {
      setTitle(nextTitle);
      if (!asset) return;
      const v = editVersionRef.current;
      const payload = { title: nextTitle, content: asset.content };
      const contentHash = Array.from(
        new Uint8Array(
          await crypto.subtle.digest("SHA-256", new TextEncoder().encode(asset.content ?? ""))
        )
      ).map(b => b.toString(16).padStart(2, "0")).join("").substring(0, 12);
      console.log(`[handleTitleChange] content hash: ${contentHash} | length: ${asset.content?.length ?? 0}`);
      updateAsset.mutate(
        { id: assetId, payload },
        { onSuccess: () => setSavedVersion(v) },
      );
    },
    [asset, assetId, updateAsset],
  );

  const handleContentChange = useCallback(
    async (content: string) => {
      if (!asset) return;
      const v = editVersionRef.current;
      const contentHash = Array.from(
        new Uint8Array(
          await crypto.subtle.digest("SHA-256", new TextEncoder().encode(content ?? ""))
        )
      ).map(b => b.toString(16).padStart(2, "0")).join("").substring(0, 12);
      console.log(`[handleContentChange] content hash: ${contentHash} | length: ${content?.length ?? 0}`);
      updateAsset.mutate(
        {
          id: assetId,
          payload: { title: title ?? asset.title, content },
        },
        { onSuccess: () => setSavedVersion(v) },
      );
    },
    [asset, assetId, title, updateAsset],
  );

  const railButtons = useMemo<RightRailButton[]>(
    () => [
      {
        id: 'settings',
        icon: 'settings',
        ariaLabel: 'Settings',
        panel: ({ close }) => <ComingSoonPanel title="Settings" onClose={close} />,
      },
    ],
    [],
  );
  useRightRailSection('asset-detail', railButtons);
  useRightRailPage('content-bank-asset', 'settings');

  if (isLoading) {
    return (
      <PageContainer>
        <PageLoader />
      </PageContainer>
    );
  }

  if (isError || !asset) {
    return (
      <PageContainer>
        <PageError header="Asset not found" />
      </PageContainer>
    );
  }

  return (
    <PageContainer variant={'fullFlex'}>
      <ScrollArea className={'flex-1 min-h-0 lg:px-6'} type={'scroll'} scrollHideDelay={350}>
          <EditPageHeader
            title={((title ?? asset.title).trim() === '' ? 'untitled' : (title ?? asset.title))}
            breadcrumbs={[{ label: 'Content Bank', to: '/content-bank' }]}
            unsaved={isDirty}
          />
          <div className={'flex flex-col items-center gap-0 relative z-0'}>
            <div className={'w-[740px] bg-white px-16 py-8 mt-2 mb-8'}>
              <AssetEditor
                initialTitle={asset.title}
                initialContent={asset.content}
                onTitleChange={handleTitleChange}
                onContentChange={handleContentChange}
                onDirty={markDirty}
              />
            </div>
          </div>
      </ScrollArea>
    </PageContainer>
  );
}
