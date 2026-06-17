import { useRef, useState } from "react";
import {
  createFileRoute,
  Outlet,
  useNavigate,
  useRouterState,
} from "@tanstack/react-router";
import { PlusIcon, UploadSimpleIcon } from "@phosphor-icons/react";
import { PageContainer } from "@/components/page-primitives/PageContainer.tsx";
import { PageHeader } from "@/components/page-primitives/PageHeader.tsx";
import { Button } from "@/components/ui/button.tsx";
import { CampaignTabBar } from "@/components/campaigns/CampaignTabBar.tsx";
import { useCreateAsset } from "@/hooks/useContent.ts";
import { useRightRailPage } from "@/hooks/useRightRailPage";
import { UploadModal } from "@/components/uploads/UploadModal";
import { useUploadStore } from "@/stores/uploadStore";
import { UPLOAD_LIMITS_LABEL } from "@/lib/assetStatus";
import { CONTENT_BANK_TABS, type ContentBankTab } from "@/lib/assetCategory";

export const Route = createFileRoute("/_authenticated/content-bank")({
  component: ContentBankLayout,
});

function activeTabFromPath(pathname: string): ContentBankTab {
  if (pathname.includes("/text")) return "text";
  if (pathname.includes("/imagery")) return "imagery";
  if (pathname.includes("/files")) return "files";
  return "all";
}

function ContentBankLayout() {
  const navigate = useNavigate();
  const createAsset = useCreateAsset();
  useRightRailPage("content-bank", null);

  // The active tab is derived from the URL rather than local state.
  const pathname = useRouterState({ select: (s) => s.location.pathname });
  const activeTab = activeTabFromPath(pathname);

  const enqueueUploads = useUploadStore((s) => s.enqueue);
  const [uploadModalOpen, setUploadModalOpen] = useState(false);
  const [isDragging, setIsDragging] = useState(false);
  const dragDepth = useRef(0);

  const hasFiles = (e: React.DragEvent) => e.dataTransfer.types.includes("Files");

  const handleDragEnter = (e: React.DragEvent) => {
    if (!hasFiles(e)) return;
    e.preventDefault();
    dragDepth.current += 1;
    setIsDragging(true);
  };

  const handleDragOver = (e: React.DragEvent) => {
    if (!hasFiles(e)) return;
    e.preventDefault();
  };

  const handleDragLeave = () => {
    dragDepth.current = Math.max(0, dragDepth.current - 1);
    if (dragDepth.current === 0) setIsDragging(false);
  };

  const handleDrop = (e: React.DragEvent) => {
    e.preventDefault();
    dragDepth.current = 0;
    setIsDragging(false);
    const files = Array.from(e.dataTransfer.files);
    if (files.length) enqueueUploads(files);
  };

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

  const handleTabSelect = (id: string) => {
    if (id === activeTab) return;
    navigate({ to: `/content-bank/${id}` });
  };

  return (
    <PageContainer variant="fullFlex">
      <div
        className="relative flex flex-col h-full min-h-0"
        onDragEnter={handleDragEnter}
        onDragOver={handleDragOver}
        onDragLeave={handleDragLeave}
        onDrop={handleDrop}
      >
        <PageHeader
          title={"Content Bank"}
          className={"pt-6"}
          actions={
            <div className="flex items-center gap-2">
              <Button onClick={() => setUploadModalOpen(true)} variant="outline" size="lg">
                <UploadSimpleIcon className="size-4" /><span>UPLOAD</span>
              </Button>
              <Button onClick={handleCreate} disabled={createAsset.isPending} size="lg">
                <PlusIcon className={"size-4"} /><span>ADD ASSET</span>
              </Button>
            </div>
          }
        />
        <CampaignTabBar
          activeTab={activeTab}
          tabs={CONTENT_BANK_TABS}
          onTabSelect={handleTabSelect}
        />
        <div className={"grid overflow-hidden h-full mt-1 px-3 lg:mt-2 lg:px-6"}>
          <Outlet />
        </div>

        {isDragging && (
          <div className="absolute inset-0 flex items-center justify-center bg-popover/90 pointer-events-none">
            <div className="flex flex-col items-center gap-2">
              <UploadSimpleIcon className="size-8 text-foreground" />
              <p className="text-sm text-foreground">Drop .md or .pdf files to upload</p>
              <p className="text-xs text-tertiary-foreground">{UPLOAD_LIMITS_LABEL}</p>
            </div>
          </div>
        )}
      </div>

      <UploadModal isOpen={uploadModalOpen} onClose={() => setUploadModalOpen(false)} />
    </PageContainer>
  );
}
