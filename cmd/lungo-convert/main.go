// lungo-convert converts a Next.js project to a Lungo project.
//
// Usage:
//
//	go run cmd/lungo-convert/main.go ./my-nextjs-app ./my-lungo-app
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Println("Usage: lungo-convert <nextjs-dir> <lungo-dir>")
		fmt.Println("  Converts a Next.js project to Lungo.")
		os.Exit(1)
	}

	src := os.Args[1]
	dst := os.Args[2]

	if _, err := os.Stat(src); err != nil {
		fmt.Printf("Error: source directory '%s' not found\n", src)
		os.Exit(1)
	}

	if _, err := os.Stat(dst); err == nil {
		fmt.Printf("Error: destination '%s' already exists\n", dst)
		os.Exit(1)
	}

	fmt.Printf("Converting Next.js → Lungo\n")
	fmt.Printf("  Source: %s\n", src)
	fmt.Printf("  Output: %s\n", dst)
	fmt.Println()

	c := &converter{
		src:      src,
		dst:      dst,
		warnings: []string{},
		apiRoutes: []apiRoute{},
	}

	c.run()
}

type apiRoute struct {
	method string
	path   string
	file   string
}

type converter struct {
	src       string
	dst       string
	warnings  []string
	apiRoutes []apiRoute
	envVars   map[string]string
}

func (c *converter) run() {
	os.MkdirAll(c.dst, 0755)
	os.MkdirAll(filepath.Join(c.dst, "app"), 0755)
	os.MkdirAll(filepath.Join(c.dst, "static"), 0755)

	// 1. Convert pages
	fmt.Println("=== Converting pages ===")
	c.convertPages()

	// 2. Convert static assets
	fmt.Println("\n=== Converting static assets ===")
	c.convertStatic()

	// 3. Read env files
	c.readEnvFiles()

	// 4. Generate main.go
	fmt.Println("\n=== Generating main.go ===")
	c.generateMainGo()

	// 5. Generate go.mod
	c.generateGoMod()

	// 6. Generate package.json (Tailwind)
	c.generatePackageJSON()

	// 7. Copy CSS
	c.convertCSS()

	// 8. Copy .env files
	c.copyEnvFiles()

	// Summary
	fmt.Println("\n=== Conversion complete ===")
	fmt.Printf("  Output: %s\n", c.dst)
	if len(c.warnings) > 0 {
		fmt.Println("\n  Manual fixes needed:")
		for _, w := range c.warnings {
			fmt.Printf("    ⚠ %s\n", w)
		}
	}
	fmt.Println("\n  Next steps:")
	fmt.Println("    cd " + c.dst)
	fmt.Println("    go mod tidy")
	fmt.Println("    npm install")
	fmt.Println("    npx @tailwindcss/cli -i app/input.css -o static/styles.css")
	fmt.Println("    LUNGO_DEV=1 go run .")
}

// ── Page conversion ──────────────────────────────────

func (c *converter) convertPages() {
	appDir := filepath.Join(c.src, "app")
	if _, err := os.Stat(appDir); err != nil {
		// Try src/ directory
		appDir = filepath.Join(c.src, "src", "app")
		if _, err := os.Stat(appDir); err != nil {
			fmt.Println("  No app/ directory found")
			return
		}
	}

	filepath.WalkDir(appDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}

		// Skip non-page files
		name := d.Name()
		if !strings.HasSuffix(name, ".tsx") && !strings.HasSuffix(name, ".jsx") && !strings.HasSuffix(name, ".ts") && !strings.HasSuffix(name, ".js") {
			return nil
		}

		// Skip API routes (handled separately)
		rel, _ := filepath.Rel(appDir, path)
		if strings.HasPrefix(rel, "api/") || strings.HasPrefix(rel, "api\\") {
			c.collectAPIRoute(path, rel)
			return nil
		}

		// Skip non-page/layout files
		base := strings.TrimSuffix(name, filepath.Ext(name))
		if base != "page" && base != "layout" && base != "loading" && base != "error" && base != "not-found" {
			// Could be a component file — check if it's in a _components or _lib dir
			if strings.Contains(rel, "_components") || strings.Contains(rel, "_lib") || strings.Contains(rel, "lib/") {
				return nil
			}
			c.warnings = append(c.warnings, "Skipped non-page file: "+rel)
			return nil
		}

		// Convert
		c.convertFile(path, rel)
		return nil
	})
}

func (c *converter) convertFile(srcPath, relPath string) {
	data, err := os.ReadFile(srcPath)
	if err != nil {
		return
	}

	content := string(data)
	original := content

	// Strip TypeScript
	content = stripTypeScript(content)

	// Convert imports
	content = convertImports(content)

	// Convert Next.js patterns
	content = convertNextPatterns(content)

	// Convert server components (async function with fetch)
	content, hasLoader := convertServerComponent(content)

	// Output as .jsx
	outRel := strings.TrimSuffix(relPath, filepath.Ext(relPath)) + ".jsx"
	outPath := filepath.Join(c.dst, "app", outRel)
	os.MkdirAll(filepath.Dir(outPath), 0755)
	os.WriteFile(outPath, []byte(content), 0644)

	status := "converted"
	if content == original {
		status = "copied (no changes)"
	}
	if hasLoader {
		status += " (+ loader)"
	}
	fmt.Printf("  %s → %s (%s)\n", relPath, outRel, status)
}

// ── TypeScript stripping ─────────────────────────────

func stripTypeScript(s string) string {
	// Remove type imports: import type { ... } from ...
	s = regexp.MustCompile(`import\s+type\s+\{[^}]*\}\s+from\s+['"][^'"]+['"];?\n?`).ReplaceAllString(s, "")

	// Remove : Type annotations from function params
	// e.g., (props: PageProps) → (props)
	s = regexp.MustCompile(`:\s*\w+(?:<[^>]*>)?(\s*[,)])`).ReplaceAllString(s, "$1")

	// Remove interface/type declarations
	s = regexp.MustCompile(`(?m)^(?:export\s+)?(?:interface|type)\s+\w+[^{]*\{[^}]*\}\n?`).ReplaceAllString(s, "")

	// Remove 'as Type' assertions
	s = regexp.MustCompile(`\s+as\s+\w+(?:<[^>]*>)?`).ReplaceAllString(s, "")

	// Remove generic type params from functions: <T>(  → (
	// Only match if preceded by function name/identifier and followed by (
	s = regexp.MustCompile(`(\w)\s*<\w+(?:\s+extends\s+[^>]+)?>\s*\(`).ReplaceAllStringFunc(s, func(m string) string {
		// Keep the leading char and opening paren, remove the generic
		re := regexp.MustCompile(`(\w)\s*<\w+(?:\s+extends\s+[^>]+)?>\s*\(`)
		return re.ReplaceAllString(m, "$1(")
	})

	// Remove React.FC, React.ReactNode etc.
	s = regexp.MustCompile(`:\s*React\.\w+`).ReplaceAllString(s, "")

	return s
}

// ── Import conversion ────────────────────────────────

func convertImports(s string) string {
	// Collect what's imported from react
	reactImports := map[string]bool{}
	re := regexp.MustCompile(`import\s+\{([^}]+)\}\s+from\s+['"]react['"];?\n?`)
	matches := re.FindAllStringSubmatch(s, -1)
	for _, m := range matches {
		for _, imp := range strings.Split(m[1], ",") {
			imp = strings.TrimSpace(imp)
			if imp != "" && imp != "React" {
				reactImports[imp] = true
			}
		}
	}
	// Remove react imports
	s = re.ReplaceAllString(s, "")
	s = regexp.MustCompile(`import\s+React\s+from\s+['"]react['"];?\n?`).ReplaceAllString(s, "")
	s = regexp.MustCompile(`import\s+\*\s+as\s+React\s+from\s+['"]react['"];?\n?`).ReplaceAllString(s, "")

	// Remove next/* imports and track what was used
	hasImage := false
	hasLink := false

	if regexp.MustCompile(`import.*from\s+['"]next/image['"]`).MatchString(s) {
		hasImage = true
		reactImports["Image"] = true
	}
	if regexp.MustCompile(`import.*from\s+['"]next/link['"]`).MatchString(s) {
		hasLink = true
		_ = hasLink
	}
	if regexp.MustCompile(`import.*useRouter.*from\s+['"]next/navigation['"]`).MatchString(s) {
		reactImports["useRouter"] = true
	}
	if regexp.MustCompile(`import.*usePathname.*from\s+['"]next/navigation['"]`).MatchString(s) {
		reactImports["useRouter"] = true // usePathname → useRouter().pathname
	}

	// Remove all next/* imports
	s = regexp.MustCompile(`import\s+\w+\s+from\s+['"]next/\w+['"];?\n?`).ReplaceAllString(s, "")
	s = regexp.MustCompile(`import\s+\{[^}]+\}\s+from\s+['"]next/\w+['"];?\n?`).ReplaceAllString(s, "")

	// Remove "use client" / "use server" directives
	s = regexp.MustCompile(`['"]use (?:client|server)['"];?\n?`).ReplaceAllString(s, "")

	// Build Lungo import
	lungoImports := []string{"h"}
	for imp := range reactImports {
		if imp != "React" && imp != "Fragment" {
			lungoImports = append(lungoImports, imp)
		}
	}
	if hasImage && !reactImports["Image"] {
		lungoImports = append(lungoImports, "Image")
	}

	// Add Lungo import at the top (after any remaining imports)
	lungoLine := "const { " + strings.Join(lungoImports, ", ") + " } = window.Lungo;\n"

	// Find where to insert (after last import or at top)
	lastImportEnd := 0
	importRe := regexp.MustCompile(`(?m)^import\s+.*;\n?`)
	locs := importRe.FindAllStringIndex(s, -1)
	if len(locs) > 0 {
		lastImportEnd = locs[len(locs)-1][1]
	}

	if lastImportEnd > 0 {
		s = s[:lastImportEnd] + "\n" + lungoLine + s[lastImportEnd:]
	} else {
		s = lungoLine + "\n" + s
	}

	return s
}

// ── Next.js pattern conversion ───────────────────────

func convertNextPatterns(s string) string {
	// <Link href="...">text</Link> → <a href="...">text</a>
	s = regexp.MustCompile(`<Link\b`).ReplaceAllString(s, "<a")
	s = regexp.MustCompile(`</Link>`).ReplaceAllString(s, "</a>")

	// Remove next/image specific props that don't apply
	s = regexp.MustCompile(`\s+fill\b`).ReplaceAllString(s, "")
	s = regexp.MustCompile(`\s+sizes="[^"]*"`).ReplaceAllString(s, "")
	s = regexp.MustCompile(`\s+quality=\{[^}]*\}`).ReplaceAllString(s, "")
	s = regexp.MustCompile(`\s+placeholder="[^"]*"`).ReplaceAllString(s, "")
	s = regexp.MustCompile(`\s+blurDataURL="[^"]*"`).ReplaceAllString(s, "")

	// usePathname() → useRouter().pathname
	s = strings.ReplaceAll(s, "usePathname()", "useRouter().pathname")

	// Remove Suspense wrapper (not needed in Lungo)
	s = regexp.MustCompile(`<Suspense[^>]*>`).ReplaceAllString(s, "")
	s = strings.ReplaceAll(s, "</Suspense>", "")

	return s
}

// ── Server component conversion ──────────────────────

func convertServerComponent(s string) (string, bool) {
	// Check for async function with fetch
	if !strings.Contains(s, "async") || !strings.Contains(s, "fetch(") {
		return s, false
	}

	// Try to extract fetch URL and convert to loader
	fetchRe := regexp.MustCompile(`(?s)const\s+\w+\s*=\s*await\s+fetch\(\s*['"]([^'"]+)['"]`)
	match := fetchRe.FindStringSubmatch(s)
	if match == nil {
		return s, false
	}

	url := match[1]

	// Add loader export
	loaderLine := fmt.Sprintf("\nexport const loader = { url: \"%s\" };\n", url)

	// Remove async keyword from the function
	s = regexp.MustCompile(`export\s+default\s+async\s+function`).ReplaceAllString(s, "export default function")

	// Remove the fetch call and .json() call
	s = fetchRe.ReplaceAllString(s, "// Data loaded via loader (was: fetch)")
	s = regexp.MustCompile(`(?s)const\s+\w+\s*=\s*await\s+\w+\.json\(\);\n?`).ReplaceAllString(s, "")

	// Insert loader before the function
	funcIdx := strings.Index(s, "export default function")
	if funcIdx > 0 {
		s = s[:funcIdx] + loaderLine + "\n" + s[funcIdx:]
	}

	return s, true
}

// ── API route collection ─────────────────────────────

func (c *converter) collectAPIRoute(path, rel string) {
	data, _ := os.ReadFile(path)
	content := string(data)

	// Extract HTTP methods
	routePath := "/" + strings.TrimSuffix(filepath.Dir(rel), "/route")
	routePath = strings.ReplaceAll(routePath, "\\", "/")
	routePath = strings.ReplaceAll(routePath, "/api", "/api")

	for _, method := range []string{"GET", "POST", "PUT", "DELETE"} {
		if strings.Contains(content, "export async function "+method) || strings.Contains(content, "export function "+method) {
			c.apiRoutes = append(c.apiRoutes, apiRoute{method: method, path: routePath, file: rel})
			fmt.Printf("  API: %s %s\n", method, routePath)
		}
	}
}

// ── Static assets ────────────────────────────────────

func (c *converter) convertStatic() {
	publicDir := filepath.Join(c.src, "public")
	if _, err := os.Stat(publicDir); err != nil {
		fmt.Println("  No public/ directory")
		return
	}

	filepath.WalkDir(publicDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(publicDir, path)
		dst := filepath.Join(c.dst, "static", rel)
		os.MkdirAll(filepath.Dir(dst), 0755)
		data, _ := os.ReadFile(path)
		os.WriteFile(dst, data, 0644)
		fmt.Printf("  %s → static/%s\n", rel, rel)
		return nil
	})
}

// ── CSS conversion ───────────────────────────────────

func (c *converter) convertCSS() {
	// Look for globals.css or global.css
	for _, name := range []string{"globals.css", "global.css", "app/globals.css", "src/app/globals.css"} {
		path := filepath.Join(c.src, name)
		if data, err := os.ReadFile(path); err == nil {
			// Write as input.css
			os.WriteFile(filepath.Join(c.dst, "app", "input.css"), data, 0644)
			fmt.Printf("\n  CSS: %s → app/input.css\n", name)
			return
		}
	}
	// Create minimal input.css
	os.WriteFile(filepath.Join(c.dst, "app", "input.css"), []byte("@import \"tailwindcss\";\n"), 0644)
	fmt.Println("\n  CSS: created minimal app/input.css")
}

// ── Env files ────────────────────────────────────────

func (c *converter) readEnvFiles() {
	c.envVars = map[string]string{}
	for _, name := range []string{".env", ".env.local", ".env.production"} {
		path := filepath.Join(c.src, name)
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || line[0] == '#' {
				continue
			}
			eq := strings.IndexByte(line, '=')
			if eq > 0 {
				key := line[:eq]
				val := line[eq+1:]
				// Strip NEXT_PUBLIC_ prefix
				key = strings.TrimPrefix(key, "NEXT_PUBLIC_")
				c.envVars[key] = val
			}
		}
	}
}

func (c *converter) copyEnvFiles() {
	if len(c.envVars) == 0 {
		return
	}
	var sb strings.Builder
	for k, v := range c.envVars {
		sb.WriteString(k + "=" + v + "\n")
	}
	sb.WriteString("LUNGO_DEV=1\n")
	os.WriteFile(filepath.Join(c.dst, ".env"), []byte(sb.String()), 0644)

	var example strings.Builder
	for k := range c.envVars {
		example.WriteString(k + "=\n")
	}
	example.WriteString("LUNGO_DEV=1\n")
	os.WriteFile(filepath.Join(c.dst, ".env.example"), []byte(example.String()), 0644)
	fmt.Println("\n  ENV: generated .env and .env.example")
}

// ── main.go generation ───────────────────────────────

func (c *converter) generateMainGo() {
	var sb strings.Builder
	sb.WriteString(`package main

import (
	"log"
	"os"

	"github.com/marcoschwartz/lungo/pkg/lungo"
)

func main() {
	dev := os.Getenv("LUNGO_DEV") == "1"

	app := lungo.New(lungo.Options{
		AppDir:       "./app",
		StaticDir:    "./static",
		Dev:          dev,
		DefaultTheme: "dark",
		Cache: &lungo.CacheOptions{
			DefaultMode:      "static",
			RevalidateSecret: os.Getenv("REVALIDATE_SECRET"),
		},
	})

	app.Use(lungo.CORS(lungo.CORSOptions{AllowOrigins: []string{"*"}}))
`)

	// Add API routes as TODOs
	if len(c.apiRoutes) > 0 {
		sb.WriteString("\n\t// ── API Routes (converted from Next.js) ──\n")
		for _, r := range c.apiRoutes {
			sb.WriteString(fmt.Sprintf("\t// TODO: Implement %s %s (was: %s)\n", r.method, r.path, r.file))
		}
		sb.WriteString("\n")
	}

	sb.WriteString(`
	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}
	log.Fatal(app.ListenAndServe(":" + port))
}
`)

	outPath := filepath.Join(c.dst, "main.go")
	os.WriteFile(outPath, []byte(sb.String()), 0644)
	fmt.Printf("  Generated main.go\n")
}

// ── go.mod generation ────────────────────────────────

func (c *converter) generateGoMod() {
	// Try to get module name from package.json
	modName := "myapp"
	pkgPath := filepath.Join(c.src, "package.json")
	if data, err := os.ReadFile(pkgPath); err == nil {
		var pkg map[string]interface{}
		if json.Unmarshal(data, &pkg) == nil {
			if name, ok := pkg["name"].(string); ok {
				modName = strings.ReplaceAll(name, "@", "")
				modName = strings.ReplaceAll(modName, "/", "-")
			}
		}
	}

	content := fmt.Sprintf(`module %s

go 1.25.1

require github.com/marcoschwartz/lungo v0.7.5

require (
	github.com/fsnotify/fsnotify v1.9.0 // indirect
	github.com/gorilla/websocket v1.5.3 // indirect
	golang.org/x/sys v0.13.0 // indirect
)
`, modName)

	os.WriteFile(filepath.Join(c.dst, "go.mod"), []byte(content), 0644)
	fmt.Printf("  Generated go.mod (module: %s)\n", modName)
}

// ── package.json generation ──────────────────────────

func (c *converter) generatePackageJSON() {
	content := `{
  "name": "lungo-app",
  "private": true,
  "dependencies": {
    "@tailwindcss/cli": "^4.2.1"
  }
}
`
	os.WriteFile(filepath.Join(c.dst, "package.json"), []byte(content), 0644)
	fmt.Println("  Generated package.json (Tailwind only)")
}
