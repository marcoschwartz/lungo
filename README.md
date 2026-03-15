# Lungo ☕

A Next.js-like framework powered by Go. No Node.js. No build step. Single binary.

```bash
go get github.com/marcoschwartz/lungo/pkg/lungo
```

## What is Lungo?

Lungo is a full-stack web framework that gives you React's developer experience with Go's performance. Write components in JSX or tagged template literals, get SSR, file-based routing, and API routes — all in a single Go binary.

**No node_modules. No Webpack. No Babel. Just Go.**

## Quick Start

```go
package main

import (
    "net/http"
    "github.com/marcoschwartz/lungo/pkg/lungo"
)

func main() {
    app := lungo.New(lungo.Options{
        AppDir: "./app",
        Dev:    true,
    })

    app.API("/api/hello", func(w http.ResponseWriter, r *http.Request) {
        w.Write([]byte(`{"message":"Hello from Go!"}`))
    })

    app.ListenAndServe(":3000")
}
```

Then create `app/page.jsx`:

```jsx
const { h, useState } = window.Lungo;

export default function Home() {
  const [count, setCount] = useState(0);

  return (
    <div>
      <h1>Count: {count}</h1>
      <button onClick={() => setCount(count + 1)}>+</button>
    </div>
  );
}
```

Run: `go run .` → Open http://localhost:3000

## Features

| Feature | Status |
|---|---|
| File-based routing (`app/page.js`) | ✅ |
| Dynamic routes (`app/blog/[slug]/page.jsx`) | ✅ |
| Nested layouts (`app/layout.js`) | ✅ |
| Server-side rendering (SSR) | ✅ |
| Streaming SSR (chunked transfer) | ✅ |
| API routes (Go handlers) | ✅ |
| Server Actions (form handlers) | ✅ |
| Static Site Generation (SSG) | ✅ |
| Data loaders (`export const loader`) | ✅ |
| SEO metadata (`export const metadata`) | ✅ |
| Middleware (CORS, Auth, Redirects) | ✅ |
| Error boundaries (`error.js`) | ✅ |
| 404 pages (`not-found.js`) | ✅ |
| Loading states (`loading.js`) | ✅ |
| HMR (hot module replacement) | ✅ |
| JSX support (Go transpiler, no Babel) | ✅ |
| Next.js compatibility layer | ✅ |
| React hooks (useState, useEffect, useMemo, useRef) | ✅ |
| Virtual DOM with diffing | ✅ |
| SVG support | ✅ |
| Dark mode | ✅ |
| Single binary deploy (`FROM scratch`) | ✅ |

## Benchmarks

Compared against Next.js 16 on the same hardware:

| Metric | Lungo | Next.js | Difference |
|---|---|---|---|
| Requests/sec | 13,138 | 2,797 | **4.7x faster** |
| Response time | 1.5ms | 19.5ms | **13x faster** |
| Memory (startup) | 12 MB | 188 MB | **15x less** |
| Memory (under load) | 22 MB | 252 MB | **11x less** |
| Build time | 823ms | 5,486ms | **6.7x faster** |
| SSG build | 13ms | 4,251ms | **327x faster** |
| Dependencies | 0.5 KB | 434 MB | **868,000x smaller** |
| Docker image | 8.5 MB | ~1 GB+ | **100x smaller** |

## File Structure

```
app/
├── page.js              → /
├── layout.js            → wraps all pages
├── loading.js           → loading state (optional)
├── error.js             → error boundary (optional)
├── not-found.js         → 404 page (optional)
├── about/
│   └── page.js          → /about
├── blog/
│   ├── page.jsx         → /blog
│   └── [slug]/
│       └── page.jsx     → /blog/:slug
└── api/                 → Go handlers (registered in main.go)
```

## JSX Support

Write standard JSX — the Go server transpiles it on the fly:

```jsx
// app/page.jsx — real JSX, zero Babel
const { h, useState } = window.Lungo;

export default function Page() {
  const [name, setName] = useState("");

  return (
    <div class="container">
      <input value={name} oninput={(e) => setName(e.target.value)} />
      <p>Hello, {name || "world"}!</p>
    </div>
  );
}
```

## Next.js Compatibility

Drop Next.js pages into your `app/` folder — the transpiler automatically converts:

- `"use client"` → stripped
- `import { useState } from "react"` → `window.Lungo`
- `className` → `class`
- `onClick` → `onclick`
- `<Link>` → `<a>`
- `<Image>` → `<img>`
- TypeScript types → stripped

## Data Loading

```jsx
export const loader = { url: "/api/posts" };

export default function Posts({ data }) {
  return <ul>{data.map(p => <li>{p.title}</li>)}</ul>;
}
```

Data is fetched server-side and injected into the HTML — no loading flash.

## Server Actions

```go
app.Action("contact", func(w http.ResponseWriter, r *http.Request) lungo.ActionResult {
    email := r.FormValue("email")
    // process...
    return lungo.ActionResult{Redirect: "/thank-you"}
})
```

```jsx
<form method="POST" action="/action/contact">
  <input name="email" type="email" />
  <button type="submit">Send</button>
</form>
```

## Static Site Generation

```go
app.Build(lungo.BuildOptions{OutputDir: "./out"})
```

Or from the command line: `go run . build`

## Docker

```dockerfile
FROM golang:1.25-alpine AS builder
WORKDIR /build
COPY . .
RUN cp runtime/lungo.js pkg/lungo/lungo_runtime.js
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /server ./example/

FROM scratch
COPY --from=builder /server /server
EXPOSE 3000
ENTRYPOINT ["/server"]
```

**8.5 MB image. Single binary. No OS. No runtime.**

## License

MIT
