import { createFileRoute } from "@tanstack/react-router";
import { ContentBankList } from "@/components/content-bank/ContentBankList.tsx";

export const Route = createFileRoute("/_authenticated/content-bank/all")({
  component: () => <ContentBankList category="all" />,
});
