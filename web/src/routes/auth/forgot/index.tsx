import { createFileRoute } from "@tanstack/react-router";

export const Route = createFileRoute("/auth/forgot/")({
  component: Forgot,
});

function Forgot() {
  return (
    <div>
      <h1>Forgot password</h1>
    </div>
  );
}
