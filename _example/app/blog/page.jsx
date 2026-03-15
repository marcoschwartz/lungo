const { h } = window.Lungo;

export const metadata = { title: "Blog", description: "Posts about our Go-powered framework." };
export const loader = { url: "/api/blog" };

export default function BlogPage({ data }) {
  const posts = Array.isArray(data) ? data : [];

  return (
    <div>
      <h1 class="text-4xl font-extrabold tracking-tight mb-2 text-gray-900 dark:text-white">Blog</h1>
      <p class="text-lg text-gray-500 dark:text-gray-400 mb-8">Dynamic routes with [slug] — just like Next.js. Data loaded via SSR.</p>

      <div class="flex flex-col gap-4">
        {posts.map(post => (
          <a href={"/blog/" + post.slug} class="block px-6 py-5 rounded-xl border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-900 hover:border-blue-300 dark:hover:border-blue-600 hover:shadow-md hover:-translate-y-0.5 transition-all no-underline">
            <div class="flex items-baseline justify-between mb-1">
              <h2 class="text-lg font-semibold text-blue-600 dark:text-blue-400">{post.title}</h2>
              <span class="text-xs text-gray-400 dark:text-gray-500">{post.date}</span>
            </div>
            <p class="text-sm text-gray-500 dark:text-gray-400">By {post.author}</p>
          </a>
        ))}
      </div>
    </div>
  );
}
