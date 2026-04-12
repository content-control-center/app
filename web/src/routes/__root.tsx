import {
  createRootRouteWithContext,
  Outlet,
  redirect,
} from "@tanstack/react-router";

import { isSetupComplete } from "../services/api/setup";
import { checkSession } from "../services/api/sessions";

export interface RouterContext {
  auth: {
    isAuthenticated: boolean;
  };
}

export const Route = createRootRouteWithContext<RouterContext>()({
  beforeLoad: async ({ location }) => {
    const isSetupRoute = location.pathname === "/setup" || location.pathname.startsWith("/setup/");
    const isAuthRoute = location.pathname === "/auth" || location.pathname.startsWith("/auth/");

    const setupDone = await isSetupComplete();

    // Setup incomplete: only /setup/* is allowed
    if (!setupDone) {
      if (!isSetupRoute) throw redirect({ to: "/setup" });
      return { auth: { isAuthenticated: false } };
    }

    // Setup complete: block /setup/*
    if (isSetupRoute) throw redirect({ to: "/" });

    // Auth routes are public — still probe so login/register can redirect away
    const authenticated = await checkSession();

    if (!authenticated && !isAuthRoute) {
      throw redirect({
        to: "/auth/login",
        search: { redirect: location.pathname },
      });
    }

    return { auth: { isAuthenticated: authenticated } };
  },
  component: () => <Outlet />,
});
