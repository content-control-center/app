import { createFileRoute } from "@tanstack/react-router";
import {PageEmpty} from "@/components/page-primitives/PageEmpty.tsx";
import {Button} from "@/components/ui/button.tsx";
import {PageContainer} from "@/components/page-primitives/PageContainer.tsx";

export const Route = createFileRoute("/_authenticated/campaigns/")({
  component: Campaigns,
});

function Campaigns() {
  return (
    <PageContainer>
      <PageEmpty
        icon={'nav_portfolios'}
        header={'No campaigns yet'}
        message={'Create your first campaign to start tracking your content publishing progress'}
        actions={
          <Button onClick={() => {}} size="lg">
            CREATE CAMPAIGN
          </Button>
        }
      />
    </PageContainer>
  );
}
