import { createFileRoute, redirect } from "@tanstack/react-router";

// /content-bank → the "All" tab.
export const Route = createFileRoute("/_authenticated/content-bank/")({
  beforeLoad: () => {
    throw redirect({ to: "/content-bank/all" });
  },
});
