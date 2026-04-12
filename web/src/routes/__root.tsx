import {
  createRootRouteWithContext,
  Outlet,
  redirect,
} from "@tanstack/react-router";

import { isSetupComplete } from "../services/api/setup";

export interface RouterContext {
  auth: {
    isAuthenticated: () => boolean;
  };
}

export const Route = createRootRouteWithContext<RouterContext>()({
  beforeLoad: async ({ location }) => {
    if (location.pathname === "/setup" || location.pathname.startsWith("/setup/")) return;
    if (!(await isSetupComplete())) {
      throw redirect({ to: "/setup" });
    }
  },
  component: () => <Outlet />,
});
