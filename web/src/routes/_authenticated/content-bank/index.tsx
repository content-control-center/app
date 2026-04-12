import { createFileRoute } from "@tanstack/react-router";
import {PageContainer} from "@/components/page-primitives/PageContainer.tsx";
import {PageEmpty} from "@/components/page-primitives/PageEmpty.tsx";
import {Button} from "@/components/ui/button.tsx";

export const Route = createFileRoute("/_authenticated/content-bank/")({
  component: ContentBank,
});

function ContentBank() {
  return (
    <PageContainer>
      <PageEmpty
        icon={'nav_portfolios'}
        header={'No content pieces yet'}
        message={'Content bank is a place to store your knowledge. Create your first content piece here'}
        actions={
          <Button onClick={() => {}} size="lg">
            CREATE CONTENT PIECE
          </Button>
        }
      />
    </PageContainer>
  )
}
