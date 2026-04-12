import "@tanstack/history";
import type { LoginPayload } from "@/types/session";

declare module "@tanstack/history" {
  interface HistoryState {
    credentials?: LoginPayload;
  }
}
