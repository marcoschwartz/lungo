const { h, useState, useEffect, useRef } = window.Lungo;

export const metadata = { title: "JSX Demo", description: "Real JSX syntax, zero Node.js — transpiled by Go." };

function Counter({ initial = 0, label = "Counter" }) {
  const [count, setCount] = useState(initial);

  return (
    <div class="inline-flex items-center gap-3 px-5 py-3 rounded-xl bg-gray-50 border border-gray-200">
      <span class="text-sm text-gray-500 mr-2">{label}:</span>
      <button
        onclick={() => setCount(count - 1)}
        class="w-9 h-9 rounded-lg border border-gray-300 bg-white hover:bg-gray-50 cursor-pointer text-lg"
      >-</button>
      <span class="text-2xl font-bold min-w-[40px] text-center tabular-nums">{count}</span>
      <button
        onclick={() => setCount(count + 1)}
        class="w-9 h-9 rounded-lg border border-gray-300 bg-white hover:bg-gray-50 cursor-pointer text-lg"
      >+</button>
    </div>
  );
}

function ToggleCard({ title, children }) {
  const [open, setOpen] = useState(false);

  return (
    <div class="border border-gray-200 rounded-xl overflow-hidden">
      <button
        onclick={() => setOpen(!open)}
        class={"w-full px-5 py-4 flex justify-between items-center text-left font-medium " + (open ? "bg-blue-50" : "bg-white hover:bg-gray-50")}
      >
        <span>{title}</span>
        <span class={"transition-transform duration-200 text-gray-400 " + (open ? "rotate-180" : "")}>▼</span>
      </button>
      {open && (
        <div class="px-5 py-4 border-t border-gray-200 text-gray-600 bg-white">
          {children}
        </div>
      )}
    </div>
  );
}

function GradientBox() {
  const [angle, setAngle] = useState(135);
  const [color1, setColor1] = useState("#3b82f6");
  const [color2, setColor2] = useState("#8b5cf6");

  const gradient = `linear-gradient(${angle}deg, ${color1}, ${color2})`;

  return (
    <div class="border border-gray-200 rounded-xl p-6 bg-white">
      <h3 class="text-lg font-bold mb-4">Gradient Builder</h3>
      <div class="w-full h-32 rounded-lg mb-4" style={{ background: gradient }} />
      <code class="block text-xs text-gray-500 bg-gray-50 rounded-lg px-3 py-2 mb-4 font-mono">{gradient}</code>
      <div class="flex flex-col gap-3">
        <label class="flex items-center gap-3 text-sm">
          <span class="w-16 text-gray-500">Angle</span>
          <input type="range" min="0" max="360" value={angle} oninput={(e) => setAngle(+e.target.value)} class="flex-1" />
          <span class="w-12 text-right font-mono text-gray-400">{angle}°</span>
        </label>
        <div class="flex gap-4">
          <label class="flex items-center gap-2 text-sm">
            <span class="text-gray-500">From</span>
            <input type="color" value={color1} oninput={(e) => setColor1(e.target.value)} class="w-8 h-8 rounded cursor-pointer" />
          </label>
          <label class="flex items-center gap-2 text-sm">
            <span class="text-gray-500">To</span>
            <input type="color" value={color2} oninput={(e) => setColor2(e.target.value)} class="w-8 h-8 rounded cursor-pointer" />
          </label>
        </div>
      </div>
    </div>
  );
}

function TypeWriter() {
  const text = "This page is written in JSX and transpiled to h() calls by a pure Go transpiler. Zero Node.js, zero Babel, zero Webpack.";
  const [displayed, setDisplayed] = useState("");
  const [done, setDone] = useState(false);

  useEffect(() => {
    let i = 0;
    const id = setInterval(() => {
      i++;
      setDisplayed(text.slice(0, i));
      if (i >= text.length) {
        clearInterval(id);
        setDone(true);
      }
    }, 30);
    return () => clearInterval(id);
  }, []);

  return (
    <div class="border border-gray-200 rounded-xl p-6 bg-white">
      <h3 class="text-lg font-bold mb-4">Typewriter Effect</h3>
      <p class="text-lg text-gray-700 font-mono min-h-[80px]">
        {displayed}<span class={"inline-block w-0.5 h-5 bg-blue-600 ml-0.5 align-middle " + (done ? "opacity-0" : "animate-pulse")} />
      </p>
    </div>
  );
}

export default function JSXDemoPage() {
  return (
    <div>
      <h1 class="text-4xl font-extrabold tracking-tight mb-2 text-gray-900">JSX Demo</h1>
      <p class="text-lg text-gray-500 mb-2 max-w-2xl">
        This page is written in <code class="px-2 py-0.5 bg-gray-100 rounded text-sm font-mono">.jsx</code> — real JSX syntax, transpiled to JavaScript by a pure Go transpiler built into the framework.
      </p>
      <p class="text-sm text-gray-400 mb-8">No Babel. No Node. No build step. The Go server transpiles on-the-fly.</p>

      <div class="grid grid-cols-1 lg:grid-cols-2 gap-6 mb-6">
        <div class="flex flex-col gap-4">
          <h2 class="text-xl font-bold">Counters</h2>
          <Counter initial={0} label="Clicks" />
          <Counter initial={100} label="Score" />
          <Counter initial={-5} label="Temperature" />
        </div>

        <GradientBox />
      </div>

      <div class="mb-6">
        <TypeWriter />
      </div>

      <div class="flex flex-col gap-2">
        <ToggleCard title="How does JSX work without Node?">
          The Go server includes a built-in JSX transpiler (~300 lines of Go). When it serves a .jsx file,
          it converts JSX syntax to h() function calls on the fly. The browser receives plain JavaScript.
        </ToggleCard>
        <ToggleCard title="Is this the same as Babel?">
          Much simpler. Babel handles TypeScript, decorators, polyfills, and hundreds of plugins.
          Our transpiler only does one thing: convert JSX tags to h() calls. That's all you need.
        </ToggleCard>
        <ToggleCard title="What about performance?">
          The transpiler runs in microseconds per file. In production, files are embedded in the binary
          and transpiled once at startup. In dev mode, they're transpiled on each request (still instant).
        </ToggleCard>
      </div>
    </div>
  );
}
