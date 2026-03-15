const { h } = window.Lungo;

export default function Loading() {
  return h`
    <div class="min-h-[40vh] flex items-center justify-center">
      <div class="flex flex-col items-center gap-4">
        <div class="w-8 h-8 border-4 border-gray-200 border-t-blue-600 rounded-full animate-spin"></div>
        <p class="text-gray-400 text-sm">Loading...</p>
      </div>
    </div>
  `;
}
