const { h, useState, useEffect, useRef } = window.Lungo;

export const metadata = { title: "Animations", description: "CSS animations and effects — arest.io style." };

// ─── Scroll-triggered animation hook ────────────────────────────

function useInView(ref) {
  const [visible, setVisible] = useState(false);

  useEffect(() => {
    if (!ref.current) return;
    const observer = new IntersectionObserver(
      ([entry]) => { if (entry.isIntersecting) setVisible(true); },
      { threshold: 0.1 }
    );
    observer.observe(ref.current);
    return () => observer.disconnect();
  }, []);

  return visible;
}

// ─── Animated counter ───────────────────────────────────────────

function AnimatedNumber({ value, label, suffix = "" }) {
  const [count, setCount] = useState(0);
  const ref = useRef(null);
  const visible = useInView(ref);

  useEffect(() => {
    if (!visible) return;
    let start = 0;
    const duration = 1500;
    const startTime = performance.now();
    const step = (now) => {
      const progress = Math.min((now - startTime) / duration, 1);
      const eased = 1 - Math.pow(1 - progress, 3);
      setCount(Math.floor(eased * value));
      if (progress < 1) requestAnimationFrame(step);
    };
    requestAnimationFrame(step);
  }, [visible]);

  return (
    <div ref={ref} class={"text-center " + (visible ? "animate-count-up" : "opacity-0")}>
      <div class="text-4xl md:text-5xl font-extrabold text-gradient tabular-nums">{count}{suffix}</div>
      <div class="text-sm text-gray-500 mt-2">{label}</div>
    </div>
  );
}

// ─── Scroll-triggered section ───────────────────────────────────

function AnimatedSection({ children, animation = "animate-fade-in-up", delay = "" }) {
  const ref = useRef(null);
  const visible = useInView(ref);

  return (
    <div ref={ref} class={visible ? animation + " " + delay : "opacity-0"}>
      {children}
    </div>
  );
}

// ─── Feature card with glow ─────────────────────────────────────

function FeatureCard({ icon, title, description, delay }) {
  return (
    <AnimatedSection animation="animate-fade-in-up" delay={delay}>
      <div class="card-glow rounded-2xl border border-gray-200 bg-white p-6 h-full">
        <div class="w-12 h-12 rounded-xl bg-blue-50 flex items-center justify-center text-2xl mb-4">
          {icon}
        </div>
        <h3 class="text-lg font-bold mb-2 text-gray-900">{title}</h3>
        <p class="text-sm text-gray-500 leading-relaxed">{description}</p>
      </div>
    </AnimatedSection>
  );
}

// ─── Typing animation ───────────────────────────────────────────

function TypingCode() {
  const lines = [
    'app := gofire.New(opts)',
    'app.API("/api/hello", handler)',
    'app.Action("contact", action)',
    'app.ListenAndServe(":3000")',
  ];
  const [displayed, setDisplayed] = useState([]);
  const [currentLine, setCurrentLine] = useState(0);
  const [currentChar, setCurrentChar] = useState(0);
  const ref = useRef(null);
  const visible = useInView(ref);

  useEffect(() => {
    if (!visible) return;
    if (currentLine >= lines.length) return;

    const id = setTimeout(() => {
      if (currentChar < lines[currentLine].length) {
        setDisplayed(prev => {
          const copy = [...prev];
          copy[currentLine] = lines[currentLine].slice(0, currentChar + 1);
          return copy;
        });
        setCurrentChar(currentChar + 1);
      } else {
        setCurrentLine(currentLine + 1);
        setCurrentChar(0);
        setDisplayed(prev => [...prev, ""]);
      }
    }, 40);
    return () => clearTimeout(id);
  }, [visible, currentLine, currentChar]);

  return (
    <div ref={ref} class="bg-gray-900 rounded-2xl p-6 font-mono text-sm overflow-hidden">
      <div class="flex gap-2 mb-4">
        <div class="w-3 h-3 rounded-full bg-red-500" />
        <div class="w-3 h-3 rounded-full bg-yellow-500" />
        <div class="w-3 h-3 rounded-full bg-green-500" />
      </div>
      <div class="text-gray-300">
        {displayed.map((line, i) => (
          <div class="flex">
            <span class="text-gray-600 w-6 text-right mr-4 select-none">{i + 1}</span>
            <span>
              <span class="text-purple-400">  </span>
              {line}
              {i === currentLine && currentLine < lines.length ? (
                <span class="inline-block w-2 h-4 bg-blue-400 ml-0.5 animate-pulse align-middle" />
              ) : null}
            </span>
          </div>
        ))}
      </div>
    </div>
  );
}

// ─── Main Page ──────────────────────────────────────────────────

export default function AnimationsPage() {
  const [mousePos, setMousePos] = useState({ x: 0, y: 0 });

  useEffect(() => {
    const handler = (e) => setMousePos({ x: e.clientX, y: e.clientY });
    window.addEventListener("mousemove", handler);
    return () => window.removeEventListener("mousemove", handler);
  }, []);

  return (
    <div>
      <section class="relative py-12 md:py-20 -mt-6 md:-mt-10 -mx-4 md:-mx-6 px-4 md:px-6 bg-grid overflow-hidden">
        <div class="absolute inset-0 bg-gradient-to-b from-blue-50/50 to-white pointer-events-none" />

        <div class="relative max-w-3xl mx-auto text-center">
          <div class="animate-fade-in-down">
            <span class="inline-flex items-center gap-2 px-4 py-1.5 rounded-full bg-blue-100 text-blue-700 text-xs font-medium mb-6">
              <span class="w-1.5 h-1.5 rounded-full bg-blue-500 animate-pulse" />
              Pure CSS Animations
            </span>
          </div>

          <h1 class="animate-fade-in-up delay-100 text-4xl md:text-6xl font-extrabold tracking-tight mb-6">
            <span>Build </span>
            <span class="text-gradient">beautiful UIs</span>
            <span> with zero JS animation libraries</span>
          </h1>

          <p class="animate-fade-in-up delay-200 text-lg md:text-xl text-gray-500 mb-8 max-w-2xl mx-auto">
            CSS keyframes, scroll-triggered reveals, gradient text, card glows, staggered entrances — all with Tailwind + our virtual DOM.
          </p>

          <div class="animate-fade-in-up delay-300 flex gap-4 justify-center flex-wrap">
            <a href="/demos" class="px-6 py-3 bg-gray-900 text-white rounded-xl hover:bg-gray-800 transition-colors font-medium no-underline">
              View Demos
            </a>
            <a href="/blog" class="px-6 py-3 border border-gray-300 text-gray-700 rounded-xl hover:border-gray-400 hover:bg-gray-50 transition-colors font-medium no-underline">
              Read Blog
            </a>
          </div>
        </div>

        <div class="animate-fade-in-scale delay-500 mt-12 max-w-xl mx-auto">
          <TypingCode />
        </div>
      </section>

      <section class="py-16">
        <AnimatedSection>
          <h2 class="text-3xl font-extrabold text-center mb-4">Why this framework?</h2>
          <p class="text-gray-500 text-center mb-12 max-w-lg mx-auto">Everything you need, nothing you don't.</p>
        </AnimatedSection>

        <div class="grid grid-cols-1 md:grid-cols-3 gap-6">
          <FeatureCard
            icon="⚡"
            title="13,000 req/s"
            description="Go's goroutine-per-request model handles massive concurrency with minimal memory."
            delay="delay-100"
          />
          <FeatureCard
            icon="📦"
            title="8.5 MB Docker"
            description="FROM scratch — just one binary. No OS, no runtime, no node_modules."
            delay="delay-200"
          />
          <FeatureCard
            icon="🔥"
            title="Zero Build Step"
            description="Write JSX, serve instantly. The Go server transpiles on the fly in microseconds."
            delay="delay-300"
          />
          <FeatureCard
            icon="🧩"
            title="React-like API"
            description="useState, useEffect, useMemo, useRef — same hooks you already know."
            delay="delay-400"
          />
          <FeatureCard
            icon="🗂️"
            title="File-Based Routing"
            description="Drop a page.jsx in a folder. Dynamic routes with [slug]. Nested layouts."
            delay="delay-500"
          />
          <FeatureCard
            icon="🌊"
            title="Streaming SSR"
            description="Chunked HTML delivery — users see content immediately while data loads."
            delay="delay-600"
          />
        </div>
      </section>

      <section class="py-16 -mx-4 md:-mx-6 px-4 md:px-6 bg-dots">
        <div class="bg-gradient-to-r from-blue-600 via-purple-600 to-pink-600 animate-gradient rounded-2xl p-12 text-center text-white">
          <AnimatedSection animation="animate-fade-in-up">
            <h2 class="text-3xl md:text-4xl font-extrabold mb-4">The numbers speak</h2>
            <p class="text-white/70 mb-12">Benchmarked against Next.js 16 on the same hardware.</p>
          </AnimatedSection>

          <div class="grid grid-cols-2 md:grid-cols-4 gap-8">
            <AnimatedNumber value={13138} label="Requests/sec" />
            <AnimatedNumber value={823} label="Build time (ms)" />
            <AnimatedNumber value={12} label="Memory (MB)" />
            <AnimatedNumber value={8} suffix=".5" label="Docker image (MB)" />
          </div>
        </div>
      </section>

      <section class="py-16">
        <div class="grid grid-cols-1 md:grid-cols-2 gap-8 items-center">
          <AnimatedSection animation="animate-slide-in-left">
            <h2 class="text-3xl font-extrabold mb-4 text-gray-900">Floating elements</h2>
            <p class="text-gray-500 mb-6">Pure CSS float animation with different delays — no JS animation library needed.</p>
          </AnimatedSection>
          <div class="flex gap-4 justify-center items-end">
            <div class="animate-float w-16 h-16 rounded-2xl bg-blue-500 shadow-lg shadow-blue-500/30" />
            <div class="animate-float delay-200 w-20 h-20 rounded-2xl bg-purple-500 shadow-lg shadow-purple-500/30" />
            <div class="animate-float delay-400 w-14 h-14 rounded-2xl bg-pink-500 shadow-lg shadow-pink-500/30" />
            <div class="animate-float delay-600 w-18 h-18 rounded-2xl bg-amber-500 shadow-lg shadow-amber-500/30" />
          </div>
        </div>
      </section>

      <section class="py-16">
        <AnimatedSection animation="animate-fade-in-up">
          <div class="text-center">
            <h2 class="text-3xl font-extrabold mb-4">
              <span class="text-gradient">Ready to ship?</span>
            </h2>
            <p class="text-gray-500 mb-8">One binary. One command. Zero dependencies.</p>
            <code class="inline-block px-6 py-3 bg-gray-900 text-green-400 rounded-xl font-mono text-sm">
              go install github.com/you/gofire@latest
            </code>
          </div>
        </AnimatedSection>
      </section>
    </div>
  );
}
