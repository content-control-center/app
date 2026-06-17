/**
 * Returns a title suitable for display, falling back to a placeholder when the
 * value is missing or blank (whitespace-only). The original, untrimmed title is
 * returned when present.
 */
export function formatTitle(
  title: string | null | undefined,
  fallback = "Untitled"
): string {
  return title && title.trim() !== "" ? title : fallback;
}
