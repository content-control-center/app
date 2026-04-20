import { createFileRoute } from "@tanstack/react-router";
import { PageContainer } from "@/components/page-primitives/PageContainer.tsx";
import { PageLoader } from "@/components/page-primitives/PageLoader.tsx";
import { PageError } from "@/components/page-primitives/PageError.tsx";
import { useAsset, useUpdateAsset } from "@/hooks/useContent.ts";
import { useCallback, useRef, useState } from "react";
import { AssetEditor } from "@/components/content-bank/AssetEditor.tsx";
import {EditPageHeader} from "@/components/page-primitives/EditPageHeader.tsx";
import { RightRail } from "@/components/page-primitives/RightRail.tsx";
import { ScrollArea } from "@/components/ui/scroll-area.tsx";

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
    (nextTitle: string) => {
      setTitle(nextTitle);
      if (!asset) return;
      const v = editVersionRef.current;
      updateAsset.mutate(
        {
          id: assetId,
          payload: { title: nextTitle, content: asset.content },
        },
        { onSuccess: () => setSavedVersion(v) },
      );
    },
    [asset, assetId, updateAsset],
  );

  const handleContentChange = useCallback(
    (content: string) => {
      if (!asset) return;
      const v = editVersionRef.current;
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
      <div className={'flex-1 min-h-0 flex flex-row'}>
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
        <RightRail
          buttons={[
            {
              id: 'settings',
              icon: 'settings',
              ariaLabel: 'Settings',
              panel: <div className="text-sm">Settings panel</div>,
            },
            {
              id: 'ai',
              icon: 'settings',
              ariaLabel: 'AI assistant',
              panel: <div className="text-sm">AI assistant panel</div>,
            },
            {
              id: 'stats',
              icon: 'settings',
              ariaLabel: 'Stats',
              panel: <div className="text-sm">Stats panel</div>,
            },
          ]}
        />
      </div>
    </PageContainer>
  );
}
