const { h, useState, useEffect } = window.Lungo;

export const metadata = { title: "Lungo — Go + Virtual DOM", description: "A React-like framework powered by Go. No Node.js. No build step." };
export const loader = { url: "/api/hello" };

function Counter({ initial = 0 }) {
  const [count, setCount] = useState(initial);

  return h`
    <div class="inline-flex items-center gap-3 px-5 py-3 rounded-xl bg-gray-50 dark:bg-gray-900 border border-gray-200 dark:border-gray-700">
      <button
        onclick=${() => setCount(count - 1)}
        class="w-10 h-10 rounded-lg border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-800 hover:bg-gray-50 dark:hover:bg-gray-700 cursor-pointer text-lg font-medium transition-colors text-gray-900 dark:text-white"
      >-</button>
      <span class="text-3xl font-bold min-w-[48px] text-center tabular-nums text-gray-900 dark:text-white">
        ${count}
      </span>
      <button
        onclick=${() => setCount(count + 1)}
        class="w-10 h-10 rounded-lg border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-800 hover:bg-gray-50 dark:hover:bg-gray-700 cursor-pointer text-lg font-medium transition-colors text-gray-900 dark:text-white"
      >+</button>
    </div>
  `;
}

function FeatureCard({ feature }) {
  return h`
    <div class="px-5 py-4 rounded-xl border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-900 hover:border-blue-300 dark:hover:border-blue-600 hover:shadow-sm transition-all">
      <span class="text-gray-700 dark:text-gray-300">${feature}</span>
    </div>
  `;
}

export default function HomePage({ data }) {
  const [mounted, setMounted] = useState(false);

  useEffect(() => {
    setMounted(true);
  }, []);

  return h`
    <div>
      <div class="mb-12">
        <h1 class="text-5xl font-extrabold tracking-tight mb-4 text-gray-900 dark:text-white">
          Lungo
        </h1>
        <p class="text-xl text-gray-500 dark:text-gray-400 mb-6 max-w-2xl">
          A React-like framework powered by Go. No Node.js. No build step.
          Just pure HTML, JS, and Go.
        </p>
        ${mounted && data?.message ? h`
          <div class="px-5 py-3 bg-green-50 dark:bg-green-900/30 border border-green-200 dark:border-green-800 rounded-xl text-green-700 dark:text-green-400 text-sm">
            Server says: ${data.message}
          </div>
        ` : null}
      </div>

      <div class="mb-12">
        <h2 class="text-2xl font-bold mb-4 text-gray-900 dark:text-white">Interactive Counter</h2>
        <p class="text-gray-500 dark:text-gray-400 mb-4">
          This counter uses useState — fully reactive, no build step needed:
        </p>
        <${Counter} initial=${0} />
      </div>

      ${data?.features ? h`
        <div>
          <h2 class="text-2xl font-bold mb-4 text-gray-900 dark:text-white">Features</h2>
          <div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3">
            ${data.features.map(f => h`<${FeatureCard} feature=${f} />`)}
          </div>
        </div>
      ` : null}
    </div>
  `;
}
