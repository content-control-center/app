import { createFileRoute, useNavigate } from "@tanstack/react-router";
import { PageContainer } from "@/components/page-primitives/PageContainer.tsx";
import { PageHeader } from "@/components/page-primitives/PageHeader.tsx";
import { PageLoader } from "@/components/page-primitives/PageLoader.tsx";
import { PageError } from "@/components/page-primitives/PageError.tsx";
import { Button } from "@/components/ui/button.tsx";
import { usePieces, useCreatePiece, useDeletePiece } from "@/hooks/useContent.ts";
import { ContentPiecesTable } from "@/components/tables/docsTable/index.tsx";
import {Icon} from "@/components/ui/icon.tsx";
import {PageGridEmptyState} from "@/components/page-primitives/PageGridEmptyState.tsx";

export const Route = createFileRoute("/_authenticated/content-bank/")({
  component: ContentBank,
});

function ContentBank() {
  const { data: pieces, isLoading, isError } = usePieces();
  const createPiece = useCreatePiece();
  const deletePiece = useDeletePiece();
  const navigate = useNavigate();
  const hasPieces = !!(pieces && pieces?.length > 0);

  const handleCreate = () => {
    createPiece.mutate(
      { title: "Untitled", content: " " },
      {
        onSuccess: (piece) => {
          navigate({ to: "/content-bank/$pieceId", params: { pieceId: piece.id } });
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
        <PageError header="Failed to load content pieces" />
      </PageContainer>
    );
  }



  return (
    <PageContainer variant="fullFlex">
      <PageHeader
        title={"Content Bank"}
        className={'pt-6'}
        actions={
          <Button onClick={handleCreate} disabled={createPiece.isPending} size="lg">
            <Icon name={'plus'} className={'size-4'} /><span>ADD DOCUMENT</span>
          </Button>
        }
      />
      <div className={'grid overflow-hidden h-full mt-1 px-3 lg:mt-2 lg:px-6'}>
        {hasPieces ? (
          <ContentPiecesTable
            pieces={pieces ?? []}
            onDelete={(id) => deletePiece.mutate(id)}
            emptyStateMessage="No content pieces yet"
            emptyStateActionLabel="Add Document"
            onEmptyStateAction={handleCreate}
          />
        ) : (
          <PageGridEmptyState
            title="No documents yet"
            subtitle="Create your first document to start building your content bank"
            actions={
              <Button onClick={handleCreate} disabled={createPiece.isPending} variant="defaultInverted">
                <Icon name={'plus'} className={'size-4 stroke-[2px]'} />
                <span>ADD DOCUMENT</span>
              </Button>
            }
          />
        )}


      </div>
    </PageContainer>
  );
}
