const { h } = window.Lungo;

export const loader = { url: "/api/blog/post" };

export default function BlogPostPage({ data, params }) {
  const post = data;

  if (!post || post.error) {
    return (
      <div class="text-center py-20">
        <h1 class="text-6xl font-extrabold text-gray-200 mb-4">404</h1>
        <p class="text-gray-500 mb-6">Post not found.</p>
        <a href="/blog" class="text-blue-600 hover:underline">Back to blog</a>
      </div>
    );
  }

  const paragraphs = post.content ? post.content.split("\n\n") : [];

  return (
    <div>
      <a href="/blog" class="text-sm text-blue-600 hover:underline mb-6 inline-block">← Back to blog</a>
      <article class="max-w-2xl">
        <div class="mb-8">
          <h1 class="text-4xl font-extrabold tracking-tight mb-3 text-gray-900">{post.title}</h1>
          <div class="flex items-center gap-3 text-sm text-gray-400">
            <span>{post.author}</span>
            <span>·</span>
            <span>{post.date}</span>
          </div>
        </div>
        <div>
          {paragraphs.map(p => (
            <p class="text-gray-600 leading-relaxed mb-4">{p}</p>
          ))}
        </div>
      </article>
    </div>
  );
}
