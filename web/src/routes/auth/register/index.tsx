import { createFileRoute } from "@tanstack/react-router";

export const Route = createFileRoute("/auth/register/")({
  component: Register,
});

function Register() {
  return (
    <div>
      <h1>Register</h1>
    </div>
  );
}
