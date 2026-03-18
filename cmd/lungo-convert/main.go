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
	appDir    string // resolved app/ directory path
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
	c.appDir = filepath.Join(c.src, "app")
	if _, err := os.Stat(c.appDir); err != nil {
		// Try src/ directory
		c.appDir = filepath.Join(c.src, "src", "app")
		if _, err := os.Stat(c.appDir); err != nil {
			fmt.Println("  No app/ directory found")
			return
		}
	}

	filepath.WalkDir(c.appDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}

		// Skip non-page files
		name := d.Name()
		if !strings.HasSuffix(name, ".tsx") && !strings.HasSuffix(name, ".jsx") && !strings.HasSuffix(name, ".ts") && !strings.HasSuffix(name, ".js") {
			return nil
		}

		// Skip API routes (handled separately)
		rel, _ := filepath.Rel(c.appDir, path)
		if strings.HasPrefix(rel, "api/") || strings.HasPrefix(rel, "api\\") {
			c.collectAPIRoute(path, rel)
			return nil
		}

		// Skip non-page/layout files (components handled via inlining)
		base := strings.TrimSuffix(name, filepath.Ext(name))
		if base != "page" && base != "layout" && base != "loading" && base != "error" && base != "not-found" {
			// Components in _components/ are inlined by the import resolver, skip silently
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

	// Inline local component imports (e.g. from "./_components/AppLayout")
	content = c.inlineLocalImports(content, srcPath)

	// Strip TypeScript
	content = stripTypeScript(content)

	// Convert imports
	content = convertImports(content)

	// Convert Next.js patterns
	content = convertNextPatterns(content)

	// Layout-specific transforms (remove <html>/<body> wrapper)
	base := strings.TrimSuffix(filepath.Base(relPath), filepath.Ext(relPath))
	if base == "layout" {
		content = convertLayout(content)
	}

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

// inlineLocalImports resolves local imports (e.g. from "./_components/Foo")
// and inlines the component code directly into the file.
func (c *converter) inlineLocalImports(content, srcPath string) string {
	// Match: import { Foo } from "./_components/Foo"
	// Match: import { Foo, Bar } from "./lib/utils"
	localImportRe := regexp.MustCompile(`import\s+\{([^}]+)\}\s+from\s+['"](\.[^'"]+)['"];?\n?`)
	matches := localImportRe.FindAllStringSubmatch(content, -1)
	if len(matches) == 0 {
		return content
	}

	srcDir := filepath.Dir(srcPath)
	var inlinedCode strings.Builder

	for _, m := range matches {
		importPath := m[2]
		// Skip react/next imports (already handled)
		if !strings.HasPrefix(importPath, ".") {
			continue
		}

		// Resolve to actual file path
		resolved := c.resolveImportPath(srcDir, importPath)
		if resolved == "" {
			continue
		}

		compData, err := os.ReadFile(resolved)
		if err != nil {
			continue
		}

		compContent := string(compData)
		rel, _ := filepath.Rel(c.appDir, resolved)

		fmt.Printf("  (inlining %s)\n", rel)

		// Strip the component file's own imports, TS, and directives
		compContent = stripTypeScript(compContent)
		compContent = regexp.MustCompile(`['"]use (?:client|server)['"];?\n?`).ReplaceAllString(compContent, "")
		// Remove all import lines from the component (they'll be merged with the parent)
		compContent = regexp.MustCompile(`(?m)^import\s+.*;\n?`).ReplaceAllString(compContent, "")
		// Convert Next.js patterns in the component too
		compContent = convertNextPatterns(compContent)
		// Remove "export" from the component's function declarations (they'll be local)
		compContent = regexp.MustCompile(`export\s+function\s+`).ReplaceAllString(compContent, "function ")
		compContent = regexp.MustCompile(`export\s+const\s+`).ReplaceAllString(compContent, "const ")

		inlinedCode.WriteString("\n// ── Inlined from " + importPath + " ──\n")
		inlinedCode.WriteString(strings.TrimSpace(compContent))
		inlinedCode.WriteString("\n\n")

		// Remove the import line from parent
		content = strings.Replace(content, m[0], "", 1)
	}

	if inlinedCode.Len() > 0 {
		// Insert inlined code before the first function/export
		funcIdx := strings.Index(content, "export default function")
		if funcIdx < 0 {
			funcIdx = strings.Index(content, "function ")
		}
		if funcIdx > 0 {
			content = content[:funcIdx] + inlinedCode.String() + content[funcIdx:]
		} else {
			content += inlinedCode.String()
		}
	}

	return content
}

// resolveImportPath finds the actual file for a relative import path.
func (c *converter) resolveImportPath(fromDir, importPath string) string {
	base := filepath.Join(fromDir, importPath)
	// Try with extensions
	for _, ext := range []string{".tsx", ".ts", ".jsx", ".js"} {
		path := base + ext
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	// Try as directory with index file
	for _, ext := range []string{".tsx", ".ts", ".jsx", ".js"} {
		path := filepath.Join(base, "index"+ext)
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}

// ── TypeScript stripping ─────────────────────────────

func stripTypeScript(s string) string {
	// Remove type imports: import type { ... } from ...
	s = regexp.MustCompile(`import\s+type\s+\{[^}]*\}\s+from\s+['"][^'"]+['"];?\n?`).ReplaceAllString(s, "")

	// Remove : Type annotations from function params
	// e.g., (props: PageProps) → (props)
	s = regexp.MustCompile(`:\s*\w+(?:<[^>]*>)?(\s*[,)])`).ReplaceAllString(s, "$1")

	// Remove complex type annotations on const/let/var declarations
	// e.g., const colorMap: Record<string, { bg: string }> = { → const colorMap = {
	s = regexp.MustCompile(`(const|let|var)\s+(\w+)\s*:[^=]+=`).ReplaceAllString(s, "$1 $2 =")

	// Remove interface/type declarations
	s = regexp.MustCompile(`(?m)^(?:export\s+)?(?:interface|type)\s+\w+[^{]*\{[^}]*\}\n?`).ReplaceAllString(s, "")

	// Remove 'as Type' assertions (but not inside JSX attributes)
	s = regexp.MustCompile(`\)\s+as\s+\w+(?:<[^>]*>)?`).ReplaceAllString(s, ")")

	// Remove generic type params from functions: <T>(  → (
	// Only match if preceded by function name/identifier and followed by (
	s = regexp.MustCompile(`(\w)\s*<\w+(?:\s+extends\s+[^>]+)?>\s*\(`).ReplaceAllStringFunc(s, func(m string) string {
		// Keep the leading char and opening paren, remove the generic
		re := regexp.MustCompile(`(\w)\s*<\w+(?:\s+extends\s+[^>]+)?>\s*\(`)
		return re.ReplaceAllString(m, "$1(")
	})

	// Remove React.FC, React.ReactNode etc.
	s = regexp.MustCompile(`:\s*React\.\w+`).ReplaceAllString(s, "")

	// Remove destructured param type annotations: { x }: { x: Type } → { x }
	// Handles ({ children }: { children: React.ReactNode }) → ({ children })
	s = regexp.MustCompile(`\}\s*:\s*\{[^}]*\}`).ReplaceAllString(s, "}")

	// Remove type annotations on destructured params with defaults: }: TypeName = {
	s = regexp.MustCompile(`\}\s*:\s*\w+\s*=`).ReplaceAllString(s, "} =")

	// Remove optional param markers: param? → param
	s = regexp.MustCompile(`(\w)\?(\s*[,)=])`).ReplaceAllString(s, "$1$2")

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
	s = regexp.MustCompile(`import\s+\{[^}]+\}\s+from\s+['"]next/[\w/]+['"];?\n?`).ReplaceAllString(s, "")
	// Remove next/font imports (e.g. import { Geist } from "next/font/google")
	s = regexp.MustCompile(`import\s+\{[^}]+\}\s+from\s+['"]next/font/\w+['"];?\n?`).ReplaceAllString(s, "")
	// Remove local CSS imports (Lungo uses static/styles.css)
	s = regexp.MustCompile(`import\s+['"]\.\/globals\.css['"];?\n?`).ReplaceAllString(s, "")
	// Remove font setup calls (const geistSans = Geist({...}))
	s = regexp.MustCompile(`(?s)const\s+\w+\s*=\s*(?:Geist|Geist_Mono)\s*\(\{[^}]*\}\);\n?`).ReplaceAllString(s, "")

	// Remove "use client" / "use server" directives
	s = regexp.MustCompile(`['"]use (?:client|server)['"];?\n?`).ReplaceAllString(s, "")

	// Build Lungo import — scan actual usage in the final content
	lungoImports := []string{"h"}
	for imp := range reactImports {
		if imp != "React" && imp != "Fragment" {
			lungoImports = append(lungoImports, imp)
		}
	}
	if hasImage && !reactImports["Image"] {
		lungoImports = append(lungoImports, "Image")
	}
	// Auto-detect hooks used by inlined components
	if strings.Contains(s, "useState(") && !reactImports["useState"] {
		lungoImports = append(lungoImports, "useState")
	}
	if strings.Contains(s, "useRouter(") && !reactImports["useRouter"] {
		lungoImports = append(lungoImports, "useRouter")
	}
	if strings.Contains(s, "useEffect(") && !reactImports["useEffect"] {
		lungoImports = append(lungoImports, "useEffect")
	}
	if strings.Contains(s, "useMemo(") && !reactImports["useMemo"] {
		lungoImports = append(lungoImports, "useMemo")
	}
	if strings.Contains(s, "useRef(") && !reactImports["useRef"] {
		lungoImports = append(lungoImports, "useRef")
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

	// Remove next/image boolean `fill` prop (not SVG fill="value")
	fillRe := regexp.MustCompile(`\s+fill([=\s/>])`)
	s = fillRe.ReplaceAllStringFunc(s, func(m string) string {
		idx := strings.Index(m, "fill") + 4
		if idx < len(m) && m[idx] == '=' {
			return m // keep fill="..." (SVG attribute)
		}
		return m[idx:] // remove standalone fill, keep trailing char
	})
	s = regexp.MustCompile(`\s+sizes="[^"]*"`).ReplaceAllString(s, "")
	s = regexp.MustCompile(`\s+quality=\{[^}]*\}`).ReplaceAllString(s, "")
	s = regexp.MustCompile(`\s+placeholder="[^"]*"`).ReplaceAllString(s, "")
	s = regexp.MustCompile(`\s+blurDataURL="[^"]*"`).ReplaceAllString(s, "")

	// usePathname() → useRouter().pathname
	s = strings.ReplaceAll(s, "usePathname()", "useRouter().pathname")

	// Remove Suspense wrapper (not needed in Lungo)
	s = regexp.MustCompile(`<Suspense[^>]*>`).ReplaceAllString(s, "")
	s = strings.ReplaceAll(s, "</Suspense>", "")

	// Convert Next.js <Script> to useEffect-based script loading
	s = convertScriptComponents(s)

	// Remove TypeScript `declare global` blocks
	s = regexp.MustCompile(`(?s)declare\s+global\s*\{.*?\}\n\n?`).ReplaceAllString(s, "")

	return s
}

// convertLayout transforms a Next.js RootLayout (wrapping <html><body>) to a Lungo layout.
// Lungo handles <html>/<body>, so the layout should just export the inner content wrapper.
func convertLayout(s string) string {
	// Replace RootLayout → Layout
	s = strings.Replace(s, "export default function RootLayout", "export default function Layout", 1)

	// Remove <html ...> and </html>
	s = regexp.MustCompile(`\s*<html[^>]*>\n?`).ReplaceAllString(s, "")
	s = regexp.MustCompile(`\s*</html>\n?`).ReplaceAllString(s, "")

	// Remove <body ...> and </body>, keep children
	s = regexp.MustCompile(`\s*<body[^>]*>\n?`).ReplaceAllString(s, "")
	s = regexp.MustCompile(`\s*</body>\n?`).ReplaceAllString(s, "")

	// Remove font variable references in classNames
	s = regexp.MustCompile(`\$\{geist\w+\.variable\}\s*`).ReplaceAllString(s, "")
	s = strings.ReplaceAll(s, "className={` antialiased`}", "")

	// Remove unused AnalyticsScript and FacebookPixel references if components were removed
	if !strings.Contains(s, "function AnalyticsScript") {
		s = regexp.MustCompile(`\s*<AnalyticsScript\s*/?>.*\n?`).ReplaceAllString(s, "")
	}
	if !strings.Contains(s, "function FacebookPixel") {
		s = regexp.MustCompile(`\s*<FacebookPixel\s*/?>.*\n?`).ReplaceAllString(s, "")
	}

	return s
}

// ── Server component conversion ──────────────────────

// convertScriptComponents converts Next.js <Script> to useEffect-based loading.
// <Script src={url} strategy="afterInteractive" /> → useEffect script injection
// <Script id="x">{`code`}</Script> → useEffect with inline eval
func convertScriptComponents(s string) string {
	if !strings.Contains(s, "<Script") {
		return s
	}

	// Convert <Script src={expr} strategy="afterInteractive" onLoad={...} onError={...} />
	// to: useEffect that creates a script element
	srcScriptRe := regexp.MustCompile(`(?s)<Script\s[^>]*src=\{([^}]+)\}[^>]*/>\n?`)
	s = srcScriptRe.ReplaceAllStringFunc(s, func(m string) string {
		srcMatch := regexp.MustCompile(`src=\{([^}]+)\}`).FindStringSubmatch(m)
		if srcMatch == nil {
			return ""
		}
		src := srcMatch[1]
		return fmt.Sprintf("      {(() => { if (typeof document !== 'undefined') { var s = document.createElement('script'); s.src = %s; s.defer = true; document.head.appendChild(s); } return null; })()}\n", src)
	})

	// Convert <Script id="x">{`code`}</Script> → inline script via useEffect pattern
	inlineScriptRe := regexp.MustCompile("(?s)<Script[^>]*>\\s*\\{`([^`]*)`\\}\\s*</Script>\\n?")
	s = inlineScriptRe.ReplaceAllStringFunc(s, func(m string) string {
		codeMatch := inlineScriptRe.FindStringSubmatch(m)
		if codeMatch == nil {
			return ""
		}
		code := strings.TrimSpace(codeMatch[1])
		// Escape for embedding in a JS string
		code = strings.ReplaceAll(code, "\\", "\\\\")
		code = strings.ReplaceAll(code, "'", "\\'")
		code = strings.ReplaceAll(code, "\n", "\\n")
		return fmt.Sprintf("      {(() => { if (typeof document !== 'undefined') { try { eval('%s') } catch(e) {} } return null; })()}\n", code)
	})

	// Remove any remaining <Script> tags that weren't matched
	s = regexp.MustCompile(`(?s)<Script[^>]*>.*?</Script>\n?`).ReplaceAllString(s, "")
	s = regexp.MustCompile(`<Script[^/]*/>\n?`).ReplaceAllString(s, "")

	return s
}

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
