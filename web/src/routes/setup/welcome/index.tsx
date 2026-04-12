import { createFileRoute, redirect } from "@tanstack/react-router";

import { isSetupComplete } from "@/services/api/setup";

import SetupWelcomePage from "./page";

export const Route = createFileRoute("/setup/welcome/")({
  beforeLoad: async () => {
    if (await isSetupComplete()) {
      throw redirect({ to: "/" });
    }
  },
  component: SetupWelcomePage,
});
