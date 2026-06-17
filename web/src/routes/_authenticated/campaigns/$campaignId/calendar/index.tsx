import { createFileRoute, redirect } from "@tanstack/react-router";
import { formatAnchor } from "@/components/campaigns/calendar/date";

// Bare /campaigns/:id/calendar → current week.
export const Route = createFileRoute(
  "/_authenticated/campaigns/$campaignId/calendar/",
)({
  beforeLoad: ({ params }) => {
    throw redirect({
      to: "/campaigns/$campaignId/calendar/$anchor/$view",
      params: {
        campaignId: params.campaignId,
        anchor: formatAnchor(new Date()),
        view: "week",
      },
    });
  },
});
