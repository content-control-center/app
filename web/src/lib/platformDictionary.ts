// Local mapping of platform IDs (sqids from the backend) to user-facing
// display info. The API is queried for IDs, publishers, cadence, and
// constraints, but display names and post-type labels live here so we
// fully control the wording the user sees.

export type PlatformPostType = {
  slug: string;
  label: string;
};

export type PlatformInfo = {
  id: string;
  name: string;
  postTypes: PlatformPostType[];
};

export const PLATFORMS: PlatformInfo[] = [
  {
    id: "AXqWG7U2qnpt",
    name: "LinkedIn",
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
