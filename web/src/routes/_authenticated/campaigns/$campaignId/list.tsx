import { createFileRoute } from "@tanstack/react-router";
import { Button } from "@/components/ui/button.tsx";
import { PlusIcon } from "@phosphor-icons/react";
import { PageGridEmptyState } from "@/components/page-primitives/PageGridEmptyState.tsx";
import { PostsTable } from "@/components/tables/postsTable";
import { useAddPost, useCampaignPosts, useDeletePost } from "@/hooks/usePosts.ts";

export const Route = createFileRoute(
  "/_authenticated/campaigns/$campaignId/list",
)({
  component: CampaignListView,
});

function CampaignListView() {
  const { campaignId } = Route.useParams();
  const { data: posts } = useCampaignPosts(campaignId);
  const deletePost = useDeletePost(campaignId);
  const addPost = useAddPost(campaignId);

  if (!posts || posts.length === 0) {
    return (
      <PageGridEmptyState
        title="No posts yet"
        subtitle="Add your first post to start building this campaign"
        actions={
          <Button variant="defaultInverted" onClick={addPost}>
            <PlusIcon className="size-4" />
            <span>ADD POST</span>
          </Button>
        }
      />
    );
  }

  return (
    <PostsTable
      posts={posts}
      campaignId={campaignId}
      onDelete={(id) => deletePost.mutate(id)}
      emptyStateMessage="No posts yet"
      emptyStateActionLabel="Add Post"
      onEmptyStateAction={addPost}
    />
  );
}
