const { h, useState, useEffect, useRef, useMemo } = window.Lungo;

export const metadata = { title: "Demos — Lungo", description: "Interactive demos pushing the limits of the framework." };

// ─── Todo App ────────────────────────────────────────────────────

function TodoApp() {
  const [todos, setTodos] = useState([]);
  const [input, setInput] = useState("");
  const [filter, setFilter] = useState("all");
  const inputRef = useRef(null);

  const addTodo = (e) => {
    e.preventDefault();
    if (!input.trim()) return;
    setTodos([...todos, { id: Date.now(), text: input, done: false }]);
    setInput("");
    if (inputRef.current) inputRef.current.focus();
  };

  const toggle = (id) => {
    setTodos(todos.map(t => t.id === id ? { ...t, done: !t.done } : t));
  };

  const remove = (id) => {
    setTodos(todos.filter(t => t.id !== id));
  };

  const filtered = useMemo(() => {
    if (filter === "active") return todos.filter(t => !t.done);
    if (filter === "done") return todos.filter(t => t.done);
    return todos;
  }, [todos, filter]);

  const remaining = todos.filter(t => !t.done).length;

  return (
    <div class="border border-gray-200 dark:border-gray-700 rounded-xl p-6 bg-white dark:bg-gray-900">
      <h3 class="text-lg font-bold mb-4">Todo App</h3>
      <form onsubmit={addTodo} class="flex gap-2 mb-4">
        <input
          ref={inputRef}
          value={input}
          oninput={(e) => setInput(e.target.value)}
          placeholder="What needs to be done?"
          class="flex-1 px-3 py-2 border border-gray-300 rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
        />
        <button type="submit" class="px-4 py-2 bg-blue-600 text-white rounded-lg text-sm hover:bg-blue-700">Add</button>
      </form>

      <div class="flex gap-2 mb-4">
        {["all", "active", "done"].map(f => (
          <button
            onclick={() => setFilter(f)}
            class={filter === f
              ? "px-3 py-1 text-xs rounded-full bg-blue-600 text-white"
              : "px-3 py-1 text-xs rounded-full bg-gray-100 dark:bg-gray-700 text-gray-600 dark:text-gray-300 hover:bg-gray-200 dark:hover:bg-gray-600"}
          >{f}</button>
        ))}
        <span class="ml-auto text-xs text-gray-400 dark:text-gray-500">{remaining} remaining</span>
      </div>

      <div class="flex flex-col gap-1">
        {filtered.map(todo => (
          <div class="flex items-center gap-3 py-2 px-3 rounded-lg hover:bg-gray-50 dark:hover:bg-gray-800 group">
            <input
              type="checkbox"
              checked={todo.done}
              onchange={() => toggle(todo.id)}
              class="w-4 h-4 rounded"
            />
            <span class={todo.done ? "line-through text-gray-400 dark:text-gray-500 flex-1" : "flex-1"}>{todo.text}</span>
            <button
              onclick={() => remove(todo.id)}
              class="text-red-400 hover:text-red-600 opacity-0 group-hover:opacity-100 text-sm"
            >✕</button>
          </div>
        ))}
        {filtered.length === 0 ? <p class="text-gray-400 dark:text-gray-500 text-sm py-4 text-center">No todos yet</p> : null}
      </div>
    </div>
  );
}

// ─── Tabs Component ─────────────────────────────────────────────

function Tabs({ tabs }) {
  const [active, setActive] = useState(0);

  return (
    <div class="border border-gray-200 dark:border-gray-700 rounded-xl overflow-hidden bg-white dark:bg-gray-900">
      <div class="flex border-b border-gray-200 dark:border-gray-700">
        {tabs.map((tab, i) => (
          <button
            onclick={() => setActive(i)}
            class={active === i
              ? "flex-1 px-4 py-3 text-sm font-medium text-blue-600 border-b-2 border-blue-600 bg-blue-50 dark:bg-blue-900/30"
              : "flex-1 px-4 py-3 text-sm font-medium text-gray-500 dark:text-gray-400 hover:text-gray-700 dark:hover:text-gray-200 hover:bg-gray-50 dark:hover:bg-gray-800"}
          >{tab.label}</button>
        ))}
      </div>
      <div class="p-6">{tabs[active].content}</div>
    </div>
  );
}

// ─── Timer / Stopwatch ──────────────────────────────────────────

function Stopwatch() {
  const [time, setTime] = useState(0);
  const [running, setRunning] = useState(false);
  const intervalRef = useRef(null);

  useEffect(() => {
    if (running) {
      intervalRef.current = setInterval(() => {
        setTime(t => t + 10);
      }, 10);
    } else {
      clearInterval(intervalRef.current);
    }
    return () => clearInterval(intervalRef.current);
  }, [running]);

  const mins = Math.floor(time / 60000);
  const secs = Math.floor((time % 60000) / 1000);
  const ms = Math.floor((time % 1000) / 10);
  const pad = (n) => String(n).padStart(2, "0");

  return (
    <div class="border border-gray-200 dark:border-gray-700 rounded-xl p-6 bg-white dark:bg-gray-900 text-center">
      <h3 class="text-lg font-bold mb-4">Stopwatch</h3>
      <div class="text-5xl font-mono font-bold mb-6 tabular-nums tracking-tight">
        {pad(mins)}:{pad(secs)}<span class="text-2xl text-gray-400 dark:text-gray-500">.{pad(ms)}</span>
      </div>
      <div class="flex gap-3 justify-center">
        <button
          onclick={() => setRunning(!running)}
          class={running
            ? "px-6 py-2 bg-red-500 text-white rounded-lg hover:bg-red-600"
            : "px-6 py-2 bg-green-500 text-white rounded-lg hover:bg-green-600"}
        >{running ? "Stop" : "Start"}</button>
        <button
          onclick={() => { setRunning(false); setTime(0); }}
          class="px-6 py-2 bg-gray-200 dark:bg-gray-600 text-gray-700 dark:text-gray-300 rounded-lg hover:bg-gray-300"
        >Reset</button>
      </div>
    </div>
  );
}

// ─── Color Picker ───────────────────────────────────────────────

function ColorPicker() {
  const [hue, setHue] = useState(200);
  const [sat, setSat] = useState(70);
  const [light, setLight] = useState(50);

  const color = "hsl(" + hue + ", " + sat + "%, " + light + "%)";

  return (
    <div class="border border-gray-200 dark:border-gray-700 rounded-xl p-6 bg-white dark:bg-gray-900">
      <h3 class="text-lg font-bold mb-4">Color Picker</h3>
      <div class="w-full h-24 rounded-lg mb-4 border border-gray-200 dark:border-gray-700" style={{ backgroundColor: color }}></div>
      <p class="text-center font-mono text-sm text-gray-500 dark:text-gray-400 mb-4">{color}</p>
      <div class="flex flex-col gap-3">
        <label class="flex items-center gap-3 text-sm">
          <span class="w-16 text-gray-500 dark:text-gray-400">Hue</span>
          <input type="range" min="0" max="360" value={hue} oninput={(e) => setHue(+e.target.value)} class="flex-1" />
          <span class="w-10 text-right font-mono text-gray-400 dark:text-gray-500">{hue}</span>
        </label>
        <label class="flex items-center gap-3 text-sm">
          <span class="w-16 text-gray-500 dark:text-gray-400">Sat</span>
          <input type="range" min="0" max="100" value={sat} oninput={(e) => setSat(+e.target.value)} class="flex-1" />
          <span class="w-10 text-right font-mono text-gray-400 dark:text-gray-500">{sat}%</span>
        </label>
        <label class="flex items-center gap-3 text-sm">
          <span class="w-16 text-gray-500 dark:text-gray-400">Light</span>
          <input type="range" min="0" max="100" value={light} oninput={(e) => setLight(+e.target.value)} class="flex-1" />
          <span class="w-10 text-right font-mono text-gray-400 dark:text-gray-500">{light}%</span>
        </label>
      </div>
    </div>
  );
}

// ─── Drag & Drop List ───────────────────────────────────────────

function DragList() {
  const [items, setItems] = useState(["Apple", "Banana", "Cherry", "Date", "Elderberry"]);
  const [dragging, setDragging] = useState(null);
  const [over, setOver] = useState(null);

  const onDragStart = (i) => { setDragging(i); };
  const onDragOver = (e, i) => { e.preventDefault(); setOver(i); };
  const onDrop = (i) => {
    if (dragging === null || dragging === i) return;
    const copy = [...items];
    const [removed] = copy.splice(dragging, 1);
    copy.splice(i, 0, removed);
    setItems(copy);
    setDragging(null);
    setOver(null);
  };
  const onDragEnd = () => { setDragging(null); setOver(null); };

  return (
    <div class="border border-gray-200 dark:border-gray-700 rounded-xl p-6 bg-white dark:bg-gray-900">
      <h3 class="text-lg font-bold mb-4">Drag & Drop List</h3>
      <div class="flex flex-col gap-1">
        {items.map((item, i) => (
          <div
            draggable="true"
            ondragstart={() => onDragStart(i)}
            ondragover={(e) => onDragOver(e, i)}
            ondrop={() => onDrop(i)}
            ondragend={onDragEnd}
            class={[
              "px-4 py-3 rounded-lg cursor-grab active:cursor-grabbing flex items-center gap-3 select-none transition-all",
              dragging === i ? "opacity-50 bg-blue-50 dark:bg-blue-900/30" : "bg-gray-50 dark:bg-gray-800 hover:bg-gray-100 dark:hover:bg-gray-700",
              over === i && dragging !== i ? "border-t-2 border-blue-400" : "border-t-2 border-transparent"
            ].join(" ")}
          >
            <span class="text-gray-400 dark:text-gray-500">☰</span>
            <span>{item}</span>
          </div>
        ))}
      </div>
      <p class="text-xs text-gray-400 dark:text-gray-500 mt-3">Drag items to reorder</p>
    </div>
  );
}

// ─── Chart.js Integration ───────────────────────────────────────

function ChartDemo() {
  const canvasRef = useRef(null);
  const chartRef = useRef(null);
  const [chartType, setChartType] = useState("bar");

  useEffect(() => {
    // Dynamically load Chart.js from ESM CDN
    const script = document.createElement("script");
    script.src = "https://cdn.jsdelivr.net/npm/chart.js@4/dist/chart.umd.min.js";
    script.onload = () => {
      renderChart(chartType);
    };
    document.head.appendChild(script);
    return () => {
      if (chartRef.current) chartRef.current.destroy();
    };
  }, []);

  useEffect(() => {
    if (window.Chart) renderChart(chartType);
  }, [chartType]);

  const renderChart = (type) => {
    if (!canvasRef.current || !window.Chart) return;
    if (chartRef.current) chartRef.current.destroy();

    chartRef.current = new window.Chart(canvasRef.current, {
      type: type,
      data: {
        labels: ["Go", "Rust", "Node.js", "Python", "Java", "C++"],
        datasets: [{
          label: "Performance Score",
          data: [95, 98, 60, 45, 70, 92],
          backgroundColor: [
            "rgba(0, 172, 215, 0.7)",
            "rgba(222, 98, 55, 0.7)",
            "rgba(104, 159, 56, 0.7)",
            "rgba(255, 193, 7, 0.7)",
            "rgba(233, 30, 99, 0.7)",
            "rgba(103, 58, 183, 0.7)",
          ],
          borderColor: [
            "rgb(0, 172, 215)",
            "rgb(222, 98, 55)",
            "rgb(104, 159, 56)",
            "rgb(255, 193, 7)",
            "rgb(233, 30, 99)",
            "rgb(103, 58, 183)",
          ],
          borderWidth: 2,
        }],
      },
      options: {
        responsive: true,
        plugins: {
          legend: { display: type !== "bar" },
        },
        scales: type === "pie" || type === "doughnut" ? {} : {
          y: { beginAtZero: true, max: 100 },
        },
      },
    });
  };

  return (
    <div class="border border-gray-200 dark:border-gray-700 rounded-xl p-6 bg-white dark:bg-gray-900">
      <div class="flex items-center justify-between mb-4">
        <h3 class="text-lg font-bold">Chart.js Integration</h3>
        <div class="flex gap-1">
          {["bar", "line", "pie", "doughnut"].map(t => (
            <button
              onclick={() => setChartType(t)}
              class={chartType === t
                ? "px-3 py-1 text-xs rounded-full bg-blue-600 text-white"
                : "px-3 py-1 text-xs rounded-full bg-gray-100 dark:bg-gray-700 text-gray-600 dark:text-gray-300 hover:bg-gray-200 dark:hover:bg-gray-600"}
            >{t}</button>
          ))}
        </div>
      </div>
      <canvas ref={canvasRef} height="200"></canvas>
    </div>
  );
}

// ─── Fetch / Async Data ─────────────────────────────────────────

function LiveClock() {
  const [time, setTime] = useState(new Date().toLocaleTimeString());

  useEffect(() => {
    const id = setInterval(() => setTime(new Date().toLocaleTimeString()), 1000);
    return () => clearInterval(id);
  }, []);

  return (
    <div class="border border-gray-200 dark:border-gray-700 rounded-xl p-6 bg-white dark:bg-gray-900 text-center">
      <h3 class="text-lg font-bold mb-2">Live Clock</h3>
      <div class="text-4xl font-mono font-bold tabular-nums text-blue-600">{time}</div>
      <p class="text-xs text-gray-400 dark:text-gray-500 mt-2">Updated every second via useEffect + setInterval</p>
    </div>
  );
}

// ─── Keyboard Events ────────────────────────────────────────────

function KeyTracker() {
  const [keys, setKeys] = useState([]);

  useEffect(() => {
    const handler = (e) => {
      setKeys(prev => [...prev.slice(-9), { key: e.key, code: e.code, time: Date.now() }]);
    };
    window.addEventListener("keydown", handler);
    return () => window.removeEventListener("keydown", handler);
  }, []);

  return (
    <div class="border border-gray-200 dark:border-gray-700 rounded-xl p-6 bg-white dark:bg-gray-900">
      <h3 class="text-lg font-bold mb-4">Keyboard Tracker</h3>
      <p class="text-sm text-gray-500 dark:text-gray-400 mb-3">Press any key:</p>
      <div class="flex flex-wrap gap-2 min-h-[48px]">
        {keys.map(k => (
          <span class="px-3 py-1 bg-gray-900 text-white rounded-lg text-sm font-mono">{k.key === " " ? "Space" : k.key}</span>
        ))}
        {keys.length === 0 ? <span class="text-gray-300 text-sm">Waiting for input...</span> : null}
      </div>
    </div>
  );
}

// ─── Main Page ──────────────────────────────────────────────────

export default function DemosPage() {
  return (
    <div>
      <h1 class="text-4xl font-extrabold tracking-tight mb-2 text-gray-900 dark:text-white">Interactive Demos</h1>
      <p class="text-lg text-gray-500 dark:text-gray-400 mb-8 max-w-2xl">
        Pushing the limits: state management, refs, effects, third-party libraries, drag & drop, keyboard events — all with zero build step.
      </p>

      <div class="grid grid-cols-1 lg:grid-cols-2 gap-6">
        <TodoApp />
        <Stopwatch />
        <ColorPicker />
        <LiveClock />
        <DragList />
        <KeyTracker />
      </div>

      <div class="mt-6">
        <ChartDemo />
      </div>

      <div class="mt-6">
        <Tabs tabs={[
          { label: "useState", content: (<p class="text-gray-600 dark:text-gray-300">All demos above use useState for reactive state. No build step, no JSX compiler — just tagged template literals.</p>) },
          { label: "useEffect", content: (<p class="text-gray-600 dark:text-gray-300">The stopwatch, live clock, and keyboard tracker use useEffect with cleanup functions for intervals and event listeners.</p>) },
          { label: "useRef", content: (<p class="text-gray-600 dark:text-gray-300">The stopwatch stores its interval ID in a ref. The Chart.js demo uses refs to access the canvas DOM element and the Chart instance.</p>) },
          { label: "useMemo", content: (<p class="text-gray-600 dark:text-gray-300">The todo list uses useMemo to filter todos without recomputing on every render.</p>) },
          { label: "Third-party", content: (<p class="text-gray-600 dark:text-gray-300">Chart.js loads dynamically via a script tag and integrates with our vdom through useRef + useEffect. Any vanilla JS library works the same way.</p>) },
        ]} />
      </div>
    </div>
  );
}
