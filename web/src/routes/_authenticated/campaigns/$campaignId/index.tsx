import { createFileRoute, redirect } from "@tanstack/react-router";
import { formatAnchor } from "@/components/campaigns/calendar/date";

// /campaigns/:id → current week of the calendar.
export const Route = createFileRoute("/_authenticated/campaigns/$campaignId/")({
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
