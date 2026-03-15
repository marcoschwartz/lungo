const { h, useState } = window.Lungo;

export const metadata = { title: "About — Lungo", description: "Learn about the Lungo framework." };

function Accordion({ title, children }) {
  const [open, setOpen] = useState(false);

  return (
    <div class="border border-gray-200 dark:border-gray-700 rounded-xl mb-2 overflow-hidden">
      <button
        onclick={() => setOpen(!open)}
        class={[
          "w-full px-5 py-4 border-none cursor-pointer flex justify-between items-center text-base font-medium text-left transition-colors text-gray-900 dark:text-white",
          open ? "bg-blue-50 dark:bg-blue-900/20" : "bg-white dark:bg-gray-900 hover:bg-gray-50 dark:hover:bg-gray-800"
        ].join(" ")}
      >
        <span>{title}</span>
        <span class={[
          "transition-transform duration-200 text-gray-400",
          open ? "rotate-180" : ""
        ].join(" ")}>
          ▼
        </span>
      </button>
      {open ? (
        <div class="px-5 py-4 border-t border-gray-200 dark:border-gray-700 text-gray-600 dark:text-gray-300 leading-relaxed bg-white dark:bg-gray-900">
          {children}
        </div>
      ) : null}
    </div>
  );
}

export default function AboutPage() {
  return (
    <div>
      <h1 class="text-4xl font-extrabold tracking-tight mb-4 text-gray-900 dark:text-white">About Lungo</h1>
      <p class="text-lg text-gray-500 dark:text-gray-400 mb-8 max-w-2xl">
        Lungo is a full-stack framework that combines the developer experience of React
        with the performance and simplicity of Go.
      </p>

      <h2 class="text-2xl font-bold mb-4 text-gray-900 dark:text-white">FAQ</h2>

      <Accordion title="Why no build step?">
        Lungo uses tagged template literals instead of JSX. This means your components
        are valid JavaScript that runs directly in the browser — no Babel, no Webpack,
        no node_modules. The syntax is nearly identical to JSX.
      </Accordion>

      <Accordion title="How does SSR work without Node.js?">
        The Go server generates an HTML shell with hydration markers. The client runtime
        then attaches event handlers and makes the page interactive. For data-dependent
        content, the server pre-fetches data from Go API handlers and injects it into
        the page as JSON.
      </Accordion>

      <Accordion title="What about streaming SSR?">
        Lungo supports chunked transfer encoding. The server sends the HTML shell
        immediately, then streams in content as data resolves. This gives users a fast
        first paint while data loads in the background.
      </Accordion>

      <Accordion title="Can I use existing Go libraries?">
        Absolutely. Your API routes are standard Go HTTP handlers. Use any database driver,
        auth library, or middleware you want. Lungo is just an http.Handler — it composes
        with the entire Go ecosystem.
      </Accordion>
    </div>
  );
}
