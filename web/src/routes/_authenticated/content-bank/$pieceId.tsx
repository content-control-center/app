import { createFileRoute } from "@tanstack/react-router";
import { PageContainer } from "@/components/page-primitives/PageContainer.tsx";
import { PageLoader } from "@/components/page-primitives/PageLoader.tsx";
import { PageError } from "@/components/page-primitives/PageError.tsx";
import { usePiece, useUpdatePiece } from "@/hooks/useContent.ts";
import { useCallback, useRef, useState } from "react";
import { ContentPieceEditor } from "@/components/content-bank/ContentPieceEditor.tsx";
import {EditPageHeader} from "@/components/page-primitives/EditPageHeader.tsx";
import { RightRail } from "@/components/page-primitives/RightRail.tsx";
import { ScrollArea } from "@/components/ui/scroll-area.tsx";

export const Route = createFileRoute("/_authenticated/content-bank/$pieceId")({
  component: ContentPiecePage,
});

function ContentPiecePage() {
  const { pieceId } = Route.useParams();
  const { data: piece, isLoading, isError } = usePiece(pieceId);
  const updatePiece = useUpdatePiece();
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
      if (!piece) return;
      const v = editVersionRef.current;
      updatePiece.mutate(
        {
          id: pieceId,
          payload: { title: nextTitle, content: piece.content },
        },
        { onSuccess: () => setSavedVersion(v) },
      );
    },
    [piece, pieceId, updatePiece],
  );

  const handleContentChange = useCallback(
    (content: string) => {
      if (!piece) return;
      const v = editVersionRef.current;
      updatePiece.mutate(
        {
          id: pieceId,
          payload: { title: title ?? piece.title, content },
        },
        { onSuccess: () => setSavedVersion(v) },
      );
    },
    [piece, pieceId, title, updatePiece],
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
        <PageError header="Content asset not found" />
      </PageContainer>
    );
  }

  return (
    <PageContainer variant={'fullFlex'}>
      <div className={'flex-1 min-h-0 flex flex-row'}>
        <ScrollArea className={'flex-1 min-h-0 lg:px-6'} type={'scroll'} scrollHideDelay={350}>
          <EditPageHeader
            title={((title ?? piece.title).trim() === '' ? 'untitled' : (title ?? piece.title))}
            breadcrumbs={[{ label: 'Content Bank', to: '/content-bank' }]}
            unsaved={isDirty}
          />
          <div className={'flex flex-col items-center gap-0 relative z-0'}>
            <div className={'w-[740px] bg-white px-16 py-8 mt-2 mb-8'}>
              <ContentPieceEditor
                initialTitle={piece.title}
                initialContent={piece.content}
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
