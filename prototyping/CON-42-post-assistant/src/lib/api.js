export function buildAssistantEndpoint(baseUrl, postId) {
  const base = baseUrl ? baseUrl.replace(/\/+$/, "") : "";
  return `${base}/api/posts/${postId}/assistant`;
}

export function buildMessagesEndpoint(baseUrl, postId) {
  const base = baseUrl ? baseUrl.replace(/\/+$/, "") : "";
  return `${base}/api/posts/${postId}/messages`;
}

export function buildVersionsEndpoint(baseUrl, postId) {
  const base = baseUrl ? baseUrl.replace(/\/+$/, "") : "";
  return `${base}/api/posts/${postId}/versions`;
}

export function buildPostEndpoint(baseUrl, postId) {
  const base = baseUrl ? baseUrl.replace(/\/+$/, "") : "";
  return `${base}/api/posts/${postId}`;
}

export function buildPostsEndpoint(baseUrl, campaignId) {
  const base = baseUrl ? baseUrl.replace(/\/+$/, "") : "";
  if (campaignId) {
    return `${base}/api/campaigns/${campaignId}/posts`;
  }
  return `${base}/api/posts`;
}
