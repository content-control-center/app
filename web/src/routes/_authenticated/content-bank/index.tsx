import { createFileRoute } from "@tanstack/react-router";

export const Route = createFileRoute("/_authenticated/content-bank/")({
  component: ContentBank,
});

function ContentBank() {
  return (
    <div>
      <h1>Content Bank</h1>
    </div>
  );
}
