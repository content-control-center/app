import type { Asset } from "@/types/content";

/** Content Bank tabs. "all" shows everything; the rest filter by content category. */
export type ContentBankTab = "all" | "text" | "imagery" | "files";

/** Derived content category for an asset, used by the tabs and the Type column. */
export type AssetCategory = Exclude<ContentBankTab, "all">;

export const CONTENT_BANK_TABS: { id: ContentBankTab; label: string }[] = [
  { id: "all", label: "ALL" },
  { id: "text", label: "TEXT" },
  { id: "imagery", label: "IMAGERY" },
  { id: "files", label: "FILES" },
];

const CATEGORY_LABEL: Record<AssetCategory, string> = {
  text: "Text",
  imagery: "Imagery",
  files: "Files",
};

/**
 * Maps a backend asset `type` to a Content Bank category.
 *
 * The backend currently only distinguishes `MD` (markdown) and `PDF`; assets
 * created in-app have a null type and are markdown text. There is no imagery
 * asset type yet (tracked separately), so nothing maps to "imagery" today and
 * the Imagery tab stays empty until the backend exposes one.
 */
export function assetCategory(asset: Pick<Asset, "type">): AssetCategory {
  switch (asset.type) {
    case "PDF":
      return "files";
    case "MD":
    default:
      return "text";
  }
}

export function categoryLabel(category: AssetCategory): string {
  return CATEGORY_LABEL[category];
}
