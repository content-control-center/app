export function PostContent({ post, description }) {
  if (!post) {
    return (
      <div className="flex h-full items-center justify-center text-xs text-slate-400">
        Select a post to view its content
      </div>
    );
  }

  return (
    <div className="flex h-full flex-col overflow-hidden bg-slate-100 p-6">
      <div className="flex-1 overflow-y-auto">
        <div className="mx-auto max-w-2xl rounded bg-white px-10 py-8 shadow-md ring-1 ring-slate-200/60"
          style={{ minHeight: "100%" }}
        >
          {post.title && (
            <h1 className="text-lg font-bold text-slate-900 mb-4 pb-3 border-b border-slate-100">
              {post.title}
            </h1>
          )}
          <div className="text-sm leading-relaxed text-slate-700 whitespace-pre-wrap">
            {description || <span className="italic text-slate-400">No content yet</span>}
          </div>
        </div>
      </div>
    </div>
  );
}
