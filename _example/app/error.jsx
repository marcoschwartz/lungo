const { h } = window.Lungo;

export default function ErrorPage({ error }) {
  return (
    <div class="min-h-[60vh] flex items-center justify-center">
      <div class="text-center">
        <h1 class="text-8xl font-extrabold text-red-100 mb-4">500</h1>
        <h2 class="text-2xl font-bold text-gray-900 mb-2">Something went wrong</h2>
        <p class="text-gray-500 mb-4">An unexpected error occurred.</p>
        {error ? (
          <pre class="text-left bg-red-50 border border-red-200 rounded-xl p-4 text-sm text-red-700 mb-8 max-w-xl mx-auto overflow-x-auto">
            {error}
          </pre>
        ) : null}
        <a href="/" class="inline-block px-6 py-3 bg-blue-600 text-white rounded-lg hover:bg-blue-700 transition-colors no-underline">
          Go Home
        </a>
      </div>
    </div>
  );
}
