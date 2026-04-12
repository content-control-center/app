import { createFileRoute, redirect } from "@tanstack/react-router";

export const Route = createFileRoute("/auth/")({
  beforeLoad: ({ context }) => {
    throw redirect({
      to: context.auth.isAuthenticated() ? "/" : "/auth/login",
    });
  },
});
