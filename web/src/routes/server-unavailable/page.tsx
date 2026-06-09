import { useState } from "react";
import { useRouter } from "@tanstack/react-router";

import { PageContainer } from "@/components/page-primitives/PageContainer";
import { PageError } from "@/components/page-primitives/PageError";
import { Button } from "@/components/ui/button";

function ServerUnavailablePage() {
  const router = useRouter();
  const [retrying, setRetrying] = useState(false);

  // Re-run the root guard: it re-probes the backend (the probe caches were
  // cleared on failure). If the server is back, the guard redirects into the
  // app; if not, we stay here.
  const retry = async () => {
    setRetrying(true);
    try {
      await router.invalidate();
    } finally {
      setRetrying(false);
    }
  };

  return (
    <PageContainer variant="fullscreen">
      <PageError
        subHeader="503"
        header="Can't reach the server"
        message={
          <>
            The app can't connect to the server right now.
            <br />
            It may be restarting or temporarily offline.
          </>
        }
        errorType="OFFLINE"
        action={
          <Button variant="outline" onClick={retry} disabled={retrying}>
            {retrying ? "Trying…" : "Try again"}
          </Button>
        }
      />
    </PageContainer>
  );
}

export default ServerUnavailablePage;
