import { createFileRoute } from "@tanstack/react-router";

import ServerUnavailablePage from "./page";

// The root guard decides whether this page is shown (server unreachable) or
// skipped (server reachable → redirect into the app), so the route itself just
// renders.
export const Route = createFileRoute("/server-unavailable/")({
  component: ServerUnavailablePage,
});
