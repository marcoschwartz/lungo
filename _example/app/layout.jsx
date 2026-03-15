const { h, useState, useRouter, useEffect } = window.Lungo;

function getInitialTheme() {
  if (typeof localStorage !== "undefined") {
    const saved = localStorage.getItem("theme");
    if (saved) return saved;
  }
  if (typeof window !== "undefined" && window.matchMedia("(prefers-color-scheme: dark)").matches) {
    return "dark";
  }
  return "light";
}

function NavLink({ href, children, onClick }) {
  const router = useRouter();
  const isActive = router.pathname === href;

  return (
    <a
      href={href}
      onclick={onClick}
      class={isActive
        ? "text-blue-600 dark:text-blue-400 font-semibold border-b-2 md:border-b-2 border-blue-600 dark:border-blue-400 px-4 py-3 md:py-2 block"
        : "text-gray-500 dark:text-gray-400 hover:text-gray-900 dark:hover:text-white border-b-2 border-transparent px-4 py-3 md:py-2 transition-colors block"}
    >
      {children}
    </a>
  );
}

function ThemeToggle({ dark, onToggle }) {
  return (
    <button
      onclick={onToggle}
      class="w-9 h-9 flex items-center justify-center rounded-lg hover:bg-gray-100 dark:hover:bg-gray-800 transition-colors text-gray-500 dark:text-gray-400"
      aria-label="Toggle theme"
    >
      {dark
        ? (<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="5"/><path d="M12 1v2M12 21v2M4.22 4.22l1.42 1.42M18.36 18.36l1.42 1.42M1 12h2M21 12h2M4.22 19.78l1.42-1.42M18.36 5.64l1.42-1.42"/></svg>)
        : (<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M21 12.79A9 9 0 1111.21 3 7 7 0 0021 12.79z"/></svg>)}
    </button>
  );
}

export default function Layout({ children }) {
  const [menuOpen, setMenuOpen] = useState(false);
  const [dark, setDark] = useState(() => getInitialTheme() === "dark");
  const router = useRouter();

  // Apply theme to <html>
  useEffect(() => {
    if (dark) {
      document.documentElement.classList.add("dark");
      localStorage.setItem("theme", "dark");
    } else {
      document.documentElement.classList.remove("dark");
      localStorage.setItem("theme", "light");
    }
  }, [dark]);

  // Listen for system theme changes
  useEffect(() => {
    const mq = window.matchMedia("(prefers-color-scheme: dark)");
    const handler = (e) => {
      if (!localStorage.getItem("theme")) {
        setDark(e.matches);
      }
    };
    mq.addEventListener("change", handler);
    return () => mq.removeEventListener("change", handler);
  }, []);

  // Close menu on navigation
  useEffect(() => {
    setMenuOpen(false);
  }, [router.pathname]);

  return (
    <div class="min-h-screen flex flex-col bg-white dark:bg-gray-950 overflow-x-hidden transition-colors duration-200">
      <nav class="border-b border-gray-200 dark:border-gray-800 bg-white dark:bg-gray-950 shadow-sm">
        <div class="flex items-center justify-between px-4 md:px-6 h-14 md:h-16">
          <a href="/" class="text-lg md:text-xl font-bold text-blue-600 dark:text-blue-400 no-underline tracking-tight shrink-0">
            Lungo
          </a>

          <div class="hidden md:flex items-center gap-1">
            <NavLink href="/">Home</NavLink>
            <NavLink href="/about">About</NavLink>
            <NavLink href="/blog">Blog</NavLink>
            <NavLink href="/posts">Posts</NavLink>
            <NavLink href="/contact">Contact</NavLink>
            <NavLink href="/demos">Demos</NavLink>
            <NavLink href="/jsx-demo">JSX</NavLink>
            <NavLink href="/live">Live</NavLink>
            <NavLink href="/animations">Anim</NavLink>
          </div>

          <div class="flex items-center gap-1">
            <ThemeToggle dark={dark} onToggle={() => setDark(!dark)} />
            <button
              onclick={() => setMenuOpen(!menuOpen)}
              class="md:hidden w-10 h-10 flex items-center justify-center rounded-lg hover:bg-gray-100 dark:hover:bg-gray-800 transition-colors text-gray-700 dark:text-gray-300"
              aria-label="Menu"
            >
              {menuOpen
                ? (<svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M18 6L6 18M6 6l12 12"/></svg>)
                : (<svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M3 12h18M3 6h18M3 18h18"/></svg>)}
            </button>
          </div>
        </div>

        {menuOpen ? (
          <div class="md:hidden border-t border-gray-100 dark:border-gray-800 bg-white dark:bg-gray-950 px-2 py-2">
            <NavLink href="/" onclick={() => setMenuOpen(false)}>Home</NavLink>
            <NavLink href="/about" onclick={() => setMenuOpen(false)}>About</NavLink>
            <NavLink href="/blog" onclick={() => setMenuOpen(false)}>Blog</NavLink>
            <NavLink href="/posts" onclick={() => setMenuOpen(false)}>Posts</NavLink>
            <NavLink href="/contact" onclick={() => setMenuOpen(false)}>Contact</NavLink>
            <NavLink href="/demos" onclick={() => setMenuOpen(false)}>Demos</NavLink>
            <NavLink href="/jsx-demo" onclick={() => setMenuOpen(false)}>JSX</NavLink>
            <NavLink href="/live" onclick={() => setMenuOpen(false)}>Live</NavLink>
            <NavLink href="/animations" onclick={() => setMenuOpen(false)}>Anim</NavLink>
          </div>
        ) : null}
      </nav>
      <main class="flex-1 py-6 md:py-10 px-4 md:px-6 max-w-4xl mx-auto w-full overflow-x-hidden">
        {children}
      </main>
      <footer class="border-t border-gray-200 dark:border-gray-800 py-6 text-center text-gray-400 dark:text-gray-600 text-xs md:text-sm px-4">
        Built with Lungo — Go + Virtual DOM, no build step
      </footer>
    </div>
  );
}
