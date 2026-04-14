import { createFileRoute, redirect } from "@tanstack/react-router";
import type { LoginPayload } from "@/types/session";

import SetupSuccessPage from "./page";

export const Route = createFileRoute("/setup/success/")({
  beforeLoad: ({ location }) => {
    const state = location.state as { credentials?: LoginPayload };
    if (!state.credentials) {
      throw redirect({ to: "/setup" });
    }
  },
  component: SetupSuccessPage,
});
