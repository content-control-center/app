import { createFileRoute, redirect } from "@tanstack/react-router";

import { isSetupComplete } from "@/services/api/setup";

export const Route = createFileRoute("/setup/")({
  beforeLoad: async () => {
    // `/setup` is a namespace, not a page. Once setup is finished the wizard
    // must not be reachable; otherwise land on the welcome step.
    if (await isSetupComplete()) {
      throw redirect({ to: "/" });
    }
    throw redirect({ to: "/setup/welcome" });
  },
});
