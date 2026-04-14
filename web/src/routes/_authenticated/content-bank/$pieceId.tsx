import { createFileRoute } from "@tanstack/react-router";
import { PageContainer } from "@/components/page-primitives/PageContainer.tsx";
import { PageLoader } from "@/components/page-primitives/PageLoader.tsx";
import { PageError } from "@/components/page-primitives/PageError.tsx";
import { usePiece, useUpdatePiece } from "@/hooks/useContent.ts";
import { useCallback, useState } from "react";
import { ContentPieceEditor } from "@/components/content-bank/ContentPieceEditor.tsx";
import {EditPageHeader} from "@/components/page-primitives/EditPageHeader.tsx";

export const Route = createFileRoute("/_authenticated/content-bank/$pieceId")({
  component: ContentPiecePage,
});

function ContentPiecePage() {
  const { pieceId } = Route.useParams();
  const { data: piece, isLoading, isError } = usePiece(pieceId);
  const updatePiece = useUpdatePiece();
  const [title, setTitle] = useState<string | null>(null);

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
        title={title ?? piece.title}
        breadcrumbs={[{ label: 'Content Bank', to: '/content-bank' }]}
      />
      <div className={'pl-6 px-6 flex flex-col items-center gap-0 shrink-0'}>
        <div className={'w-[740px] px-16 pt-8 bg-white'}>
        <ContentPieceEditor
          initialContent={piece.content}
          onContentChange={handleContentChange}
          onTitleChange={setTitle}
        />
        </div>
      </div>
    </PageContainer>
  );
}
