const { h, useState, useEffect } = window.Lungo;

export const metadata = { title: "Posts — Lungo", description: "Blog posts loaded from a Go API." };
export const loader = { url: "/api/posts" };

function PostCard({ post }) {
  return h`
    <div class="px-6 py-5 rounded-xl border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-900 hover:border-blue-300 dark:hover:border-blue-600 hover:shadow-md hover:-translate-y-0.5 transition-all cursor-pointer">
      <h3 class="text-lg font-semibold mb-2 text-blue-600 dark:text-blue-400">
        ${post.title}
      </h3>
      <p class="text-gray-500 dark:text-gray-400 text-sm">
        ${post.excerpt}
      </p>
    </div>
  `;
}

export default function PostsPage({ data }) {
  const posts = Array.isArray(data) ? data : [];

  return h`
    <div>
      <h1 class="text-4xl font-extrabold tracking-tight mb-4 text-gray-900 dark:text-white">Posts</h1>
      <p class="text-lg text-gray-500 dark:text-gray-400 mb-8 max-w-2xl">
        These posts are loaded from a Go API handler via the loader pattern.
      </p>

      <div class="flex flex-col gap-3">
        ${posts.map(post => h`<${PostCard} post=${post} />`)}
      </div>

      ${posts.length === 0 ? h`
        <p class="text-gray-400 italic">No posts found.</p>
      ` : null}
    </div>
  `;
}
