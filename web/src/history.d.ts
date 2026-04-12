import "@tanstack/history";
import type { LoginPayload } from "@/services/api/sessions";

declare module "@tanstack/history" {
  interface HistoryState {
    credentials?: LoginPayload;
  }
}
