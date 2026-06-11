import { createFileRoute, Outlet } from "@tanstack/react-router";
import { useMemo } from "react";
import { SidebarProvider, SidebarInset } from "@/components/ui/sidebar";
import { AppSidebar } from "@/components/layout/AppSidebar";
import { Backdrop } from "@/components/ui/backdrop";
import { SecondaryNavbarContainer } from "@/components/layout/SecondaryNavbar";
import { CampaignsListContent } from "@/components/layout/CampaignsListContent";
import { OverlayOutlet } from "@/components/layout/OverlayOutlet";
import { UploadTracker } from "@/components/uploads/UploadTracker";
import { GLOBAL_RAIL_SECTION_ID, RightRail } from "@/components/page-primitives/RightRail";
import { useSettingsStore } from "@/stores/settingsStore";
import { ZIndex } from "@/config/zIndex";
import { useRightRailSection } from "@/hooks/useRightRailSection";
import type { RightRailButton } from "@/stores/rightRailStore";
import { AIAssistantPanel, StatsPanel } from "@/components/rail-panels/ComingSoonPanel";
import { ChartBar, TrendUp } from "@phosphor-icons/react";

export const Route = createFileRoute("/_authenticated")({
  component: AuthenticatedLayout,
});

function AuthenticatedLayout() {
  const sidebarCollapsed = useSettingsStore((s) => s.sidebarCollapsed);
  const setSidebarCollapsed = useSettingsStore((s) => s.setSidebarCollapsed);
  const isSecondaryNavbarOpen = useSettingsStore((s) => s.isSecondaryNavbarOpen);
  const closeSecondaryNavbar = useSettingsStore((s) => s.closeSecondaryNavbar);

  const globalRailButtons = useMemo<RightRailButton[]>(
    () => [
      {
        id: "ai",
        icon: ChartBar,
        ariaLabel: "AI assistant",
        persistent: true,
        panel: ({ close }) => <AIAssistantPanel onClose={close} />,
      },
      {
        id: "stats",
        icon: TrendUp,
        ariaLabel: "Stats",
        persistent: true,
        panel: ({ close }) => <StatsPanel onClose={close} />,
      },
    ],
    [],
  );
  useRightRailSection(GLOBAL_RAIL_SECTION_ID, globalRailButtons);

  return (
    <SidebarProvider
      open={!sidebarCollapsed}
      onOpenChange={(open) => setSidebarCollapsed(!open)}
    >
      <AppSidebar />
      <SidebarInset className="min-w-0">
        <Outlet />
      </SidebarInset>
      <RightRail />
      <Backdrop
        open={isSecondaryNavbarOpen}
        onClick={closeSecondaryNavbar}
        zIndex={ZIndex.secondaryNavbarOverlay}
      />
      <SecondaryNavbarContainer>
        <CampaignsListContent />
      </SecondaryNavbarContainer>
      <OverlayOutlet />
      <UploadTracker />
    </SidebarProvider>
  );
}
