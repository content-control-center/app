import { createFileRoute, redirect } from "@tanstack/react-router";

import SetupSuccessPage from "./page";

export const Route = createFileRoute("/setup/success/")({
  beforeLoad: ({ location }) => {
    if (!location.state.credentials) {
      throw redirect({ to: "/setup" });
    }
  },
  component: SetupSuccessPage,
});
