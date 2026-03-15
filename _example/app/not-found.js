const { h } = window.Lungo;

export default function NotFound() {
  return h`
    <div class="min-h-[60vh] flex items-center justify-center">
      <div class="text-center">
        <h1 class="text-8xl font-extrabold text-gray-200 mb-4">404</h1>
        <h2 class="text-2xl font-bold text-gray-900 mb-2">Page Not Found</h2>
        <p class="text-gray-500 mb-8">The page you're looking for doesn't exist.</p>
        <a href="/" class="inline-block px-6 py-3 bg-blue-600 text-white rounded-lg hover:bg-blue-700 transition-colors no-underline">
          Go Home
        </a>
      </div>
    </div>
  `;
}
