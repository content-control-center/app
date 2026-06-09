import {
  createRootRouteWithContext,
  Link,
  Outlet,
  redirect,
} from "@tanstack/react-router";

import { PageContainer } from "../components/page-primitives/PageContainer";
import { PageError } from "../components/page-primitives/PageError";
import { Button } from "../components/ui/button";
import { ServerUnavailableError } from "../services/api/errors";
import { isSetupComplete } from "../services/api/setup";
import { checkSession } from "../services/api/sessions";
import { getMe } from "../services/api/users";
import { useAuthStore } from "../stores/authStore";

const SERVER_DOWN_PATH = "/server-unavailable";

export interface RouterContext {
  auth: {
    isAuthenticated: boolean;
  };
}

export const Route = createRootRouteWithContext<RouterContext>()({
  beforeLoad: async ({ location }) => {
    const isSetupRoute = location.pathname === "/setup" || location.pathname.startsWith("/setup/");
    const isAuthRoute = location.pathname === "/auth" || location.pathname.startsWith("/auth/");
    const isServerDownRoute = location.pathname === SERVER_DOWN_PATH;

    try {
      const setupDone = await isSetupComplete();

      // We reached the backend. If we were parked on the outage page waiting
      // for it to recover, send the user back into the app.
      if (isServerDownRoute) throw redirect({ to: "/" });

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

      // Hydrate auth store if session is valid but store is empty (e.g. fresh tab)
      if (authenticated && !useAuthStore.getState().user) {
        const user = await getMe();
        useAuthStore.getState().setUser(user);
      }

      return { auth: { isAuthenticated: authenticated } };
    } catch (err) {
      // The server is unreachable — distinct from a fresh install, which
      // returns a real response. Show the dedicated outage page instead of
      // misrouting the user into the first-run setup wizard.
      if (err instanceof ServerUnavailableError) {
        if (isServerDownRoute) return { auth: { isAuthenticated: false } };
        throw redirect({ to: SERVER_DOWN_PATH });
      }
      // Redirects and any other error propagate unchanged.
      throw err;
    }
  },
  component: () => <Outlet />,
  notFoundComponent: () => (
    <PageContainer variant="fullscreen">
      <PageError
        subHeader="404"
        header="Page not found"
        message="The page you're looking for doesn't exist or has been moved."
        errorType="NOT FOUND"
        action={
          <Link to="/">
            <Button variant="outline">Go home</Button>
          </Link>
        }
      />
    </PageContainer>
  ),
});
