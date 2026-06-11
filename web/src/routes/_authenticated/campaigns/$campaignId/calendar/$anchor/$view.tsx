import {
  createFileRoute,
  redirect,
  useNavigate,
} from "@tanstack/react-router";
import { Button } from "@/components/ui/button.tsx";
import { PlusIcon } from "@phosphor-icons/react";
import { PageGridEmptyState } from "@/components/page-primitives/PageGridEmptyState.tsx";
import { WeeklyCalendar } from "@/components/campaigns/calendar/WeeklyCalendar";
import { formatAnchor, parseAnchor } from "@/components/campaigns/calendar/date";
import { useAddPost, useCampaignPosts } from "@/hooks/usePosts.ts";

export const Route = createFileRoute(
  "/_authenticated/campaigns/$campaignId/calendar/$anchor/$view",
)({
  beforeLoad: ({ params }) => {
    // Normalize malformed anchors / unsupported views to the current week.
    const parsed = parseAnchor(params.anchor);
    if (!parsed || params.view !== "week") {
      throw redirect({
        to: "/campaigns/$campaignId/calendar/$anchor/$view",
        params: {
          campaignId: params.campaignId,
          anchor: formatAnchor(parsed ?? new Date()),
          view: "week",
        },
      });
    }
  },
  component: CalendarView,
});

function CalendarView() {
  const { campaignId, anchor } = Route.useParams();
  const navigate = useNavigate();
  const { data: posts } = useCampaignPosts(campaignId);
  const addPost = useAddPost(campaignId);
  const anchorDate = parseAnchor(anchor) ?? new Date();

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
    <WeeklyCalendar
      campaignId={campaignId}
      posts={posts}
      anchor={anchorDate}
      onAnchorChange={(d) =>
        navigate({
          to: "/campaigns/$campaignId/calendar/$anchor/$view",
          params: { campaignId, anchor: formatAnchor(d), view: "week" },
        })
      }
    />
  );
}
