import { createFileRoute } from "@tanstack/react-router";
import { PageContainer } from "@/components/page-primitives/PageContainer.tsx";
import { PageLoader } from "@/components/page-primitives/PageLoader.tsx";
import { PageError } from "@/components/page-primitives/PageError.tsx";
import { usePiece, useUpdatePiece } from "@/hooks/useContent.ts";
import { useCallback } from "react";
import { ContentPieceEditor } from "@/components/content-bank/ContentPieceEditor.tsx";
import {EditPageHeader} from "@/components/page-primitives/EditPageHeader.tsx";

export const Route = createFileRoute("/_authenticated/content-bank/$pieceId")({
  component: ContentPiecePage,
});

function ContentPiecePage() {
  const { pieceId } = Route.useParams();
  const { data: piece, isLoading, isError } = usePiece(pieceId);
  const updatePiece = useUpdatePiece();

  const handleContentChange = useCallback(
    (content: string) => {
      if (!piece) return;
      updatePiece.mutate({
        id: pieceId,
        payload: { title: piece.title, content },
      });
    },
    [piece, pieceId, updatePiece],
  );

  if (isLoading) {
    return (
      <PageContainer>
        <PageLoader />
      </PageContainer>
    );
  }

  if (isError || !piece) {
    return (
      <PageContainer>
        <PageError header="Content piece not found" />
      </PageContainer>
    );
  }

  return (
    <PageContainer>
      <EditPageHeader
        title={piece.title}
        breadcrumbs={[{ label: 'Content Bank', to: '/content-bank' }]}
      />
      <div className={'pl-16 px-6 flex flex-col gap-0 shrink-0'}>
        <ContentPieceEditor
          initialContent={piece.content}
          onContentChange={handleContentChange}
        />
      </div>
    </PageContainer>
  );
}
