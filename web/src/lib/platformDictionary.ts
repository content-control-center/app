// Local mapping of platform IDs (sqids from the backend) to user-facing
// display info. The API is queried for IDs, publishers, cadence, and
// constraints, but display names and post-type labels live here so we
// fully control the wording the user sees.

import type { Icon } from "@phosphor-icons/react";
import { FacebookLogoIcon, InstagramLogoIcon, LinkedinLogoIcon, ThreadsLogoIcon, XLogoIcon, YoutubeLogoIcon } from "@phosphor-icons/react";

import type { Platform, PlatformPublisher } from "@/types/campaigns";

export type PlatformPostType = {
  slug: string;
  label: string;
};

export type PlatformInfo = {
  id: string;
  name: string;
  icon: Icon;
  // Official brand color, hard-coded so the icon renders in its native hue
  // wherever it appears across the app.
  color: string;
  postTypes: PlatformPostType[];
};

export const PLATFORMS: PlatformInfo[] = [
  {
    id: "AXqWG7U2qnpt",
    name: "LinkedIn",
    icon: LinkedinLogoIcon,
    color: "#0A66C2",
    postTypes: [
      { slug: "text-post", label: "Text post" },
      { slug: "image-post", label: "Image post" },
      { slug: "carousel", label: "Carousel" },
      { slug: "video", label: "Video" },
      { slug: "article", label: "Article" },
      { slug: "poll", label: "Poll" },
      { slug: "newsletter", label: "Newsletter" },
      { slug: "event", label: "Event" },
      { slug: "live-video", label: "Live video" },
    ],
  },
  {
    id: "8S8bWQTG6qD",
    name: "YouTube",
    icon: YoutubeLogoIcon,
    color: "#FF0000",
    postTypes: [
      { slug: "video", label: "Video" },
      { slug: "short", label: "Short" },
      { slug: "live-stream", label: "Live stream" },
      { slug: "premiere", label: "Premiere" },
      { slug: "community-post", label: "Community post" },
      { slug: "podcast", label: "Podcast" },
    ],
  },
  {
    id: "zBU1zqVICGfk",
    name: "Facebook",
    icon: FacebookLogoIcon,
    color: "#1877F2",
    postTypes: [
      { slug: "text-post", label: "Text post" },
      { slug: "image-post", label: "Image post" },
      { slug: "video", label: "Video" },
      { slug: "reel", label: "Reel" },
      { slug: "story", label: "Story" },
      { slug: "live-video", label: "Live video" },
      { slug: "carousel", label: "Carousel" },
      { slug: "poll", label: "Poll" },
      { slug: "event", label: "Event" },
      { slug: "link-post", label: "Link post" },
    ],
  },
  {
    id: "81mUCmc2xsKd",
    name: "X (Twitter)",
    icon: XLogoIcon,
    color: "#000000",
    postTypes: [
      { slug: "text-post", label: "Text post" },
      { slug: "image-post", label: "Image post" },
      { slug: "video", label: "Video" },
      { slug: "long-form-post", label: "Long-form post" },
      { slug: "poll", label: "Poll" },
      { slug: "space", label: "Space" },
      { slug: "thread", label: "Thread" },
    ],
  },
  {
    id: "pQ4yxT3SuE57",
    name: "Threads",
    icon: ThreadsLogoIcon,
    color: "#000000",
    postTypes: [
      { slug: "text-post", label: "Text post" },
      { slug: "image-post", label: "Image post" },
      { slug: "carousel", label: "Carousel" },
      { slug: "video", label: "Video" },
      { slug: "poll", label: "Poll" },
      { slug: "gif-post", label: "GIF post" },
    ],
  },
  {
    id: "rzgpTkARLH0L",
    name: "Instagram",
    icon: InstagramLogoIcon,
    color: "#E4405F",
    postTypes: [
      { slug: "image-post", label: "Image post" },
      { slug: "carousel", label: "Carousel" },
      { slug: "reel", label: "Reel" },
      { slug: "story", label: "Story" },
      { slug: "live-video", label: "Live video" },
      { slug: "broadcast-channel", label: "Broadcast channel" },
      { slug: "collaborative-post", label: "Collaborative post" },
      { slug: "guide", label: "Guide" },
    ],
  },
];

const BY_ID: Map<string, PlatformInfo> = new Map(PLATFORMS.map((p) => [p.id, p]));

export function getPlatformInfo(id: string): PlatformInfo | undefined {
  return BY_ID.get(id);
}

export function unionSupportedSlugs(
  publishers: { supported_post_types: string[] }[],
): Set<string> {
  const out = new Set<string>();
  for (const p of publishers) for (const slug of p.supported_post_types) out.add(slug);
  return out;
}

export function partitionPostTypesBySupport(
  postTypes: PlatformPostType[],
  supportedSlugs: Set<string>,
): { supported: PlatformPostType[]; unsupported: PlatformPostType[] } {
  const supported: PlatformPostType[] = [];
  const unsupported: PlatformPostType[] = [];
  for (const pt of postTypes) {
    (supportedSlugs.has(pt.slug) ? supported : unsupported).push(pt);
  }
  return { supported, unsupported };
}

// A resolved view of a platform: the dictionary metadata joined with the
// publisher state from the API. The post-type universe is bounded by what
// at least one publisher supports — dictionary-only entries are excluded.
export type PlatformView = {
  platform: Platform;
  info: PlatformInfo;
  // post types supported by at least one publisher (connected or not)
  allowed: PlatformPostType[];
  // post types supported by at least one CONNECTED publisher
  available: PlatformPostType[];
  // allowed but not currently available (publisher exists, not connected)
  unavailable: PlatformPostType[];
  publishers: PlatformPublisher[];
  connectedPublishers: PlatformPublisher[];
  connectedPublisherName: string | null;
};

export function buildPlatformView(
  platform: Platform,
  info: PlatformInfo,
): PlatformView {
  const publishers = platform.publishers ?? [];
  const connectedPublishers = publishers.filter((p) => p.connected);
  const allowedSlugs = unionSupportedSlugs(publishers);
  const availableSlugs = unionSupportedSlugs(connectedPublishers);
  const allowed = info.postTypes.filter((pt) => allowedSlugs.has(pt.slug));
  const available = allowed.filter((pt) => availableSlugs.has(pt.slug));
  const unavailable = allowed.filter((pt) => !availableSlugs.has(pt.slug));
  return {
    platform,
    info,
    allowed,
    available,
    unavailable,
    publishers,
    connectedPublishers,
    connectedPublisherName: connectedPublishers[0]?.name ?? null,
  };
}

export function buildPlatformViews(platforms: Platform[]): PlatformView[] {
  return platforms.flatMap((platform) => {
    const info = getPlatformInfo(platform.id);
    return info ? [buildPlatformView(platform, info)] : [];
  });
}

export function getPostTypeLabel(platformId: string, slug: string): string {
  return (
    getPlatformInfo(platformId)?.postTypes.find((pt) => pt.slug === slug)
      ?.label ?? slug
  );
}
