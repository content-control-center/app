import { createFileRoute } from "@tanstack/react-router";

export const Route = createFileRoute("/_authenticated/campaigns/")({
  component: Campaigns,
});

function Campaigns() {
  return (
    <div>
      <h1>Campaigns</h1>
    </div>
  );
}
