import { createFileRoute, redirect } from "@tanstack/react-router";

import { isSetupComplete } from "@/services/api/setup";

import SetupAdminPage from "./page";

export const Route = createFileRoute("/setup/admin/")({
  beforeLoad: async () => {
    if (await isSetupComplete()) {
      throw redirect({ to: "/" });
    }
  },
  component: SetupAdminPage,
});
