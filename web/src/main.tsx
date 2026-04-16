import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { RouterProvider, createRouter } from "@tanstack/react-router";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { routeTree } from "./routeTree.gen";
import { CAMPAIGN_TYPES_KEY } from "./hooks/useCampaigns";
import { TAGS_KEY } from "./hooks/useTags";
import { PLATFORMS_KEY } from "./hooks/usePlatforms";
import { listCampaignTypes } from "./services/api/campaigns";
import { listTags } from "./services/api/tags";
import { listPlatforms } from "./services/api/platforms";
import "./index.css";

const FIVE_MINUTES = 1000 * 60 * 5;

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 30_000,
      refetchOnWindowFocus: false,
    },
  },
});

queryClient.prefetchQuery({ queryKey: CAMPAIGN_TYPES_KEY, queryFn: listCampaignTypes, staleTime: Infinity });
queryClient.prefetchQuery({ queryKey: TAGS_KEY, queryFn: listTags, staleTime: FIVE_MINUTES });
queryClient.prefetchQuery({ queryKey: PLATFORMS_KEY, queryFn: listPlatforms, staleTime: Infinity });

const router = createRouter({
  routeTree,
  context: {
    auth: { isAuthenticated: false },
  },
});

declare module "@tanstack/react-router" {
  interface Register {
    router: typeof router;
  }
}


createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  </StrictMode>
);
