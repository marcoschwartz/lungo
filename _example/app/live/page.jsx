const { h, useState, useEffect, useRef } = window.Lungo;

export const metadata = { title: "Live Dashboard", description: "Real-time server metrics via SSE." };

function Gauge({ label, value, unit, color, max = 100 }) {
  const pct = Math.min(100, Math.max(0, (value / max) * 100));

  return (
    <div class="border border-gray-200 rounded-xl p-5 bg-white">
      <div class="flex justify-between items-baseline mb-2">
        <span class="text-sm text-gray-500">{label}</span>
        <span class={"text-2xl font-bold tabular-nums " + color}>{value}<span class="text-sm text-gray-400 ml-1">{unit}</span></span>
      </div>
      <div class="w-full h-2 bg-gray-100 rounded-full overflow-hidden">
        <div class="h-full rounded-full transition-all duration-500" style={{ width: pct + "%", backgroundColor: pct > 80 ? "#ef4444" : pct > 50 ? "#f59e0b" : "#22c55e" }} />
      </div>
    </div>
  );
}

function Sparkline({ data, color, height = 60 }) {
  if (data.length < 2) return <div style={{ height: height + "px" }} />;

  const max = Math.max(...data);
  const min = Math.min(...data);
  const range = max - min || 1;
  const w = 300;
  const h_val = height;

  const points = data.map((v, i) => {
    const x = (i / (data.length - 1)) * w;
    const y = h_val - ((v - min) / range) * (h_val - 4);
    return x + "," + y;
  }).join(" ");

  const areaPoints = points + " " + w + "," + h_val + " 0," + h_val;

  return (
    <div class="w-full">
      <svg viewBox={"0 0 " + w + " " + h_val} class="w-full" style={{ height: height + "px" }}>
        <polygon points={areaPoints} fill={color} opacity="0.1" />
        <polyline points={points} fill="none" stroke={color} stroke-width="2" stroke-linecap="round" stroke-linejoin="round" />
      </svg>
    </div>
  );
}

function EventLog({ events }) {
  return (
    <div class="border border-gray-200 rounded-xl bg-white overflow-hidden">
      <div class="px-5 py-3 border-b border-gray-200 bg-gray-50">
        <h3 class="text-sm font-semibold text-gray-700">Event Log</h3>
      </div>
      <div class="max-h-64 overflow-y-auto font-mono text-xs">
        {events.map((e, i) => (
          <div class={"px-5 py-2 border-b border-gray-50 flex gap-4 " + (i === 0 ? "bg-blue-50" : "")}>
            <span class="text-gray-400 w-20 shrink-0">{e.time}</span>
            <span class="text-gray-600">CPU {e.cpu}% | MEM {e.memory}% | {e.rps} req/s | {e.latency}ms</span>
          </div>
        ))}
        {events.length === 0 && (
          <div class="px-5 py-8 text-center text-gray-400">Connecting...</div>
        )}
      </div>
    </div>
  );
}

export default function LivePage() {
  const [connected, setConnected] = useState(false);
  const [metrics, setMetrics] = useState(null);
  const [history, setHistory] = useState([]);
  const [cpuHistory, setCpuHistory] = useState([]);
  const [rpsHistory, setRpsHistory] = useState([]);
  const eventSourceRef = useRef(null);

  useEffect(() => {
    const es = new EventSource("/api/sse");
    eventSourceRef.current = es;

    es.addEventListener("connected", () => {
      setConnected(true);
    });

    es.addEventListener("metrics", (e) => {
      const data = JSON.parse(e.data);
      setMetrics(data);
      setHistory((prev) => [data, ...prev].slice(0, 50));
      setCpuHistory((prev) => [...prev, data.cpu].slice(-30));
      setRpsHistory((prev) => [...prev, data.rps].slice(-30));
    });

    es.onerror = () => {
      setConnected(false);
    };

    return () => {
      es.close();
    };
  }, []);

  return (
    <div>
      <div class="flex items-center gap-3 mb-6">
        <h1 class="text-4xl font-extrabold tracking-tight text-gray-900">Live Dashboard</h1>
        <span class={"inline-flex items-center gap-1.5 px-3 py-1 rounded-full text-xs font-medium " + (connected ? "bg-green-100 text-green-700" : "bg-red-100 text-red-700")}>
          <span class={"w-2 h-2 rounded-full " + (connected ? "bg-green-500 animate-pulse" : "bg-red-500")} />
          {connected ? "Live" : "Disconnected"}
        </span>
      </div>
      <p class="text-gray-500 mb-8">
        Real-time server metrics streamed via Server-Sent Events (SSE) from a Go handler.
      </p>

      <div class="grid grid-cols-2 lg:grid-cols-4 gap-4 mb-6">
        <Gauge label="CPU" value={metrics ? metrics.cpu : 0} unit="%" color="text-blue-600" />
        <Gauge label="Memory" value={metrics ? metrics.memory : 0} unit="%" color="text-purple-600" />
        <Gauge label="Requests" value={metrics ? metrics.rps : 0} unit="/s" color="text-green-600" max={1500} />
        <Gauge label="Latency" value={metrics ? metrics.latency : 0} unit="ms" color="text-amber-600" max={20} />
      </div>

      <div class="grid grid-cols-1 lg:grid-cols-2 gap-4 mb-6">
        <div class="border border-gray-200 rounded-xl p-5 bg-white">
          <h3 class="text-sm font-semibold text-gray-700 mb-3">CPU Usage (30s)</h3>
          <Sparkline data={cpuHistory} color="#3b82f6" />
        </div>
        <div class="border border-gray-200 rounded-xl p-5 bg-white">
          <h3 class="text-sm font-semibold text-gray-700 mb-3">Requests/sec (30s)</h3>
          <Sparkline data={rpsHistory} color="#22c55e" />
        </div>
      </div>

      <EventLog events={history} />
    </div>
  );
}
