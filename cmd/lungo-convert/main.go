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
	favicon      string // detected favicon path (e.g. "/static/favicon.png")
	title        string // detected site title from metadata
	hasAnalytics bool   // detected OmniKit analytics script
	hasFBPixel   bool   // detected Facebook pixel
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

	// 2b. Detect favicon
	c.detectFavicon()

	// 2c. Extract site title from layout metadata
	c.extractSiteTitle()

	// 3. Read env files
	c.readEnvFiles()

	// 4. Generate main.go (uses favicon, title, env vars)
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

	// 9. Generate Dockerfile
	c.generateDockerfile()

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

	// Strip inter-span whitespace inside <pre><code> blocks
	content = compactCodeSpans(content)

	// Replace HTML entities that JSX would decode
	content = strings.ReplaceAll(content, "&apos;", "'")
	content = strings.ReplaceAll(content, "&quot;", `"`)
	content = strings.ReplaceAll(content, "&amp;", "&")
	content = strings.ReplaceAll(content, "&copy;", "\u00A9")
	content = strings.ReplaceAll(content, "&rarr;", "\u2192")
	content = strings.ReplaceAll(content, "&larr;", "\u2190")
	content = strings.ReplaceAll(content, "&mdash;", "\u2014")
	content = strings.ReplaceAll(content, "&ndash;", "\u2013")
	content = strings.ReplaceAll(content, "&nbsp;", "\u00A0")

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

		// Detect tracking scripts — flag for HeadExtra generation
		if strings.Contains(compContent, "omnkit") || strings.Contains(compContent, "omnikit") {
			c.hasAnalytics = true
			fmt.Printf("  (detected analytics in %s)\n", rel)
		}
		if strings.Contains(compContent, "fbq") || strings.Contains(compContent, "FacebookPixel") {
			c.hasFBPixel = true
			fmt.Printf("  (detected Facebook pixel in %s)\n", rel)
		}

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
	// e.g., (props: PageProps) → (props), (val: number | string) → (val)
	// Only match type names (uppercase or known TS types), not object values like `: 0,`
	tsTypes := `(?:[A-Z]\w*|string|number|boolean|any|void|null|undefined|never|object|unknown)`
	s = regexp.MustCompile(`:\s*`+tsTypes+`(?:<[^>]*>)?(?:\s*\|\s*`+tsTypes+`(?:<[^>]*>)?)*(?:\[\])?(\s*[,)])`).ReplaceAllString(s, "$1")

	// Remove complex type annotations on const/let/var declarations
	// e.g., const colorMap: Record<string, { bg: string }> = { → const colorMap = {
	s = regexp.MustCompile(`(const|let|var)\s+(\w+)\s*:[^=]+=`).ReplaceAllString(s, "$1 $2 =")

	// Remove interface/type declarations (including nested braces)
	s = stripInterfacesAndTypes(s)

	// Remove 'as Type' assertions — only after ) and only for capitalized type names or { }
	// e.g., (value) as { value: number } → (value)
	// e.g., (data as Product[]) → (data)
	// Must NOT match natural text like "as you", "as needed"
	s = regexp.MustCompile(`\)\s+as\s+\{[^}]*\}`).ReplaceAllString(s, ")")
	s = regexp.MustCompile(`\)\s+as\s+[A-Z]\w*(?:<[^>]*>)?(?:\[\])?`).ReplaceAllString(s, ")")

	// Remove generic type params: useState<number[]>(...)  → useState(...)
	// Matches <...> when preceded by identifier and followed by (
	s = regexp.MustCompile(`(\w)\s*<[^>]*>\s*\(`).ReplaceAllStringFunc(s, func(m string) string {
		re := regexp.MustCompile(`(\w)\s*<[^>]*>\s*\(`)
		return re.ReplaceAllString(m, "$1(")
	})

	// Remove function return type annotations: ): Type {  → ) {
	s = regexp.MustCompile(`\)\s*:\s*\w+(?:<[^>]*>)?\s*\{`).ReplaceAllString(s, ") {")

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

// stripInterfacesAndTypes removes interface and type declarations, handling nested braces.
func stripInterfacesAndTypes(s string) string {
	re := regexp.MustCompile(`(?m)^(?:export\s+)?(?:interface|type)\s+\w+`)
	for {
		loc := re.FindStringIndex(s)
		if loc == nil {
			break
		}
		// Find the opening brace
		braceIdx := strings.Index(s[loc[0]:], "{")
		if braceIdx < 0 {
			// type alias without braces: type Foo = string | number;
			semiIdx := strings.Index(s[loc[0]:], ";")
			if semiIdx >= 0 {
				end := loc[0] + semiIdx + 1
				if end < len(s) && s[end] == '\n' {
					end++
				}
				s = s[:loc[0]] + s[end:]
			} else {
				nlIdx := strings.Index(s[loc[0]:], "\n")
				if nlIdx >= 0 {
					s = s[:loc[0]] + s[loc[0]+nlIdx+1:]
				} else {
					s = s[:loc[0]]
				}
			}
			continue
		}
		// Match nested braces
		start := loc[0] + braceIdx
		depth := 0
		end := start
		for end < len(s) {
			if s[end] == '{' {
				depth++
			} else if s[end] == '}' {
				depth--
				if depth == 0 {
					end++
					break
				}
			}
			end++
		}
		// Skip trailing newline
		if end < len(s) && s[end] == '\n' {
			end++
		}
		s = s[:loc[0]] + s[end:]
	}
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

	// Remove next/image boolean `fill` prop — only as a standalone JSX attribute
	// before /> or > (not in text content like "fill forms")
	// Match: fill followed by /> or > or another attribute
	fillRe := regexp.MustCompile(`(\s)fill(\s*/>|\s*>|\s+\w+=)`)
	s = fillRe.ReplaceAllStringFunc(s, func(m string) string {
		idx := strings.Index(m, "fill") + 4
		if idx < len(m) && m[idx] == '=' {
			return m // keep fill="..." (SVG attribute)
		}
		// Remove standalone fill prop, keep what follows
		return m[:strings.Index(m, "fill")] + m[idx:]
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

	// Remove TypeScript `declare global` blocks (with nested braces)
	for {
		idx := strings.Index(s, "declare global")
		if idx < 0 {
			break
		}
		braceStart := strings.Index(s[idx:], "{")
		if braceStart < 0 {
			break
		}
		braceStart += idx
		depth := 0
		end := braceStart
		for end < len(s) {
			if s[end] == '{' {
				depth++
			} else if s[end] == '}' {
				depth--
				if depth == 0 {
					end++
					break
				}
			}
			end++
		}
		for end < len(s) && (s[end] == '\n' || s[end] == '\r') {
			end++
		}
		s = s[:idx] + s[end:]
	}

	// Convert process.env.NEXT_PUBLIC_X → (window.__ENV && window.__ENV.X) || ""
	// Strip NEXT_PUBLIC_ prefix from the env var name
	s = regexp.MustCompile(`process\.env\.NEXT_PUBLIC_(\w+)`).ReplaceAllString(s, `(window.__ENV && window.__ENV.$1 || "")`)
	// Also handle process.env.NODE_ENV
	s = strings.ReplaceAll(s, `process.env.NODE_ENV`, `"production"`)
	// Any remaining process.env.X
	s = regexp.MustCompile(`process\.env\.(\w+)`).ReplaceAllString(s, `(window.__ENV && window.__ENV.$1 || "")`)

	return s
}

// convertLayout transforms a Next.js RootLayout (wrapping <html><body>) to a Lungo layout.
// Lungo handles <html>/<body>, so the layout should just export the inner content wrapper.
func convertLayout(s string) string {
	// Replace RootLayout → Layout
	s = strings.Replace(s, "export default function RootLayout", "export default function Layout", 1)

	// If the default Layout just wraps an inlined component (e.g. AppLayout),
	// promote that component to be the default Layout directly and remove the wrapper.
	// Detect: the export default function's body contains <ComponentName>{children}</ComponentName>
	defaultFuncIdx := strings.Index(s, "export default function Layout(")
	if defaultFuncIdx >= 0 {
		// Extract the body of the default export
		bodyStart := strings.Index(s[defaultFuncIdx:], "{")
		if bodyStart >= 0 {
			bodyStart += defaultFuncIdx
			// Find the component being wrapped: <Name>{children}</Name> or <Name>\n  {children}\n</Name>
			wrapRe := regexp.MustCompile(`<(\w+)>\s*\{?\s*children\s*\}?\s*</\w+>`)
			bodySection := s[bodyStart:]
			if m := wrapRe.FindStringSubmatch(bodySection); m != nil {
				wrappedName := m[1]
				// Check it's a local function (inlined component), not an HTML tag
				if strings.Contains(s, "function "+wrappedName+"(") {
					// Promote: rename the inlined function to be the default export
					s = strings.Replace(s, "function "+wrappedName+"(", "export default function Layout(", 1)

					// Find the end of the promoted Layout function (match braces from its body)
					layoutStart := strings.Index(s, "export default function Layout(")
					if layoutStart >= 0 {
						// Find the opening { of the function body (after params)
						parenEnd := strings.Index(s[layoutStart:], ")")
						if parenEnd >= 0 {
							searchFrom := layoutStart + parenEnd
							braceStart := strings.Index(s[searchFrom:], "{")
							if braceStart >= 0 {
								pos := searchFrom + braceStart
								depth := 0
								for pos < len(s) {
									if s[pos] == '{' { depth++ }
									if s[pos] == '}' { depth--; if depth == 0 { pos++; break } }
									pos++
								}
								// Keep everything up to end of Layout, remove the rest
								// (orphaned analytics/pixel functions + old default export)
								s = s[:pos] + "\n"
							}
						}
					}
				}
			}
		}
	}

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

	// If the default Layout function returns multiple sibling elements,
	// wrap in a fragment. Detect by checking if there are JSX elements
	// after the first closing tag inside the return().
	// Simpler approach: find the Layout's return and wrap content in <>...</>
	layoutReturnRe := regexp.MustCompile(`(?s)(export default function Layout\([^)]*\)\s*\{\s*return\s*\()(.+?)(\);\s*\})`)
	s = layoutReturnRe.ReplaceAllStringFunc(s, func(m string) string {
		parts := layoutReturnRe.FindStringSubmatch(m)
		if parts == nil {
			return m
		}
		body := strings.TrimSpace(parts[2])
		// Check if body has multiple root elements (not already wrapped in fragment)
		if !strings.HasPrefix(body, "<>") && !strings.HasPrefix(body, "<div") {
			// Count root-level elements — if there's content after the first element's closing tag
			// Just wrap in fragment to be safe
			body = "<>\n        " + body + "\n      </>"
		}
		return parts[1] + body + parts[3]
	})

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

// compactCodeSpans removes whitespace between <span> elements inside <pre><code> blocks.
// The original source has each span on a separate indented line, which renders as visible
// whitespace inside <pre>. This collapses them onto one line.
func compactCodeSpans(s string) string {
	lines := strings.Split(s, "\n")
	var result []string
	i := 0
	for i < len(lines) {
		line := lines[i]
		trimmed := strings.TrimSpace(line)

		// Detect <code> followed by span lines
		if (trimmed == "<code>" || strings.HasPrefix(trimmed, "<code>")) && !strings.Contains(trimmed, "</code>") {
			// Check if next lines are spans
			j := i + 1
			hasSpans := false
			for j < len(lines) {
				t := strings.TrimSpace(lines[j])
				if t == "</code>" || strings.HasPrefix(t, "</code>") {
					break
				}
				if strings.HasPrefix(t, "<span") {
					hasSpans = true
				}
				j++
			}

			if hasSpans && j < len(lines) {
				// Collect indent from the <code> line
				indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
				// Compact: join all spans without inter-line whitespace
				var compacted strings.Builder
				compacted.WriteString(indent + "<code>")
				for k := i + 1; k < j; k++ {
					compacted.WriteString(strings.TrimSpace(lines[k]))
				}
				compacted.WriteString("</code>")
				result = append(result, compacted.String())
				i = j + 1 // skip past </code>
				continue
			}
		}

		result = append(result, line)
		i++
	}
	return strings.Join(result, "\n")
}

func convertServerComponent(s string) (string, bool) {
	hasAsync := strings.Contains(s, "export default async function")
	// Also detect server components by the presence of async helper functions with fetch
	hasServerFetch := strings.Contains(s, "async function") && strings.Contains(s, "fetch(")

	if !hasAsync && !hasServerFetch {
		return s, false
	}

	// Strip async from the default export
	s = regexp.MustCompile(`export\s+default\s+async\s+function`).ReplaceAllString(s, "export default function")

	// Find all fetch URLs in the file
	fetchRe := regexp.MustCompile(`fetch\(\s*['"]([^'"]+)['"]`)
	fetchURLRe := regexp.MustCompile(`fetch\(\s*` + "`([^`]+)`")
	var fetchURLs []string
	for _, m := range fetchRe.FindAllStringSubmatch(s, -1) {
		fetchURLs = append(fetchURLs, m[1])
	}
	for _, m := range fetchURLRe.FindAllStringSubmatch(s, -1) {
		fetchURLs = append(fetchURLs, m[1])
	}

	// Remove await expressions (keep the expression after await)
	s = regexp.MustCompile(`\bawait\s+`).ReplaceAllString(s, "")

	// Add { data } param to the default export if not already present
	s = regexp.MustCompile(`export default function (\w+)\(\)`).ReplaceAllString(s, "export default function $1({ data })")

	if len(fetchURLs) == 0 {
		// Complex case: fetch URLs are dynamic (variables, template literals)
		// Generate a loader with /api/<pagename> and a TODO comment
		pageName := "data"
		nameRe := regexp.MustCompile(`export default function (\w+)`)
		if m := nameRe.FindStringSubmatch(s); m != nil {
			name := strings.TrimSuffix(m[1], "Page")
			pageName = strings.ToLower(name)
		}
		loaderLine := fmt.Sprintf("\nexport const loader = { url: \"/api/%s\" };\n", pageName)
		comment := fmt.Sprintf("// TODO: Add server-side API handler in main.go for /api/%s\n", pageName)
		funcIdx := strings.Index(s, "export default function")
		if funcIdx > 0 {
			s = s[:funcIdx] + comment + loaderLine + "\n" + s[funcIdx:]
		}
		return s, true
	}

	if len(fetchURLs) == 1 {
		// Simple case: single fetch → direct loader
		loaderLine := fmt.Sprintf("\nexport const loader = { url: \"%s\" };\n", fetchURLs[0])
		funcIdx := strings.Index(s, "export default function")
		if funcIdx > 0 {
			s = s[:funcIdx] + loaderLine + "\n" + s[funcIdx:]
		}

		// Remove the fetch call and .json() call
		s = fetchRe.ReplaceAllString(s, "// Data loaded via loader (was: fetch(\""+fetchURLs[0]+"\")")
		s = regexp.MustCompile(`const\s+\w+\s*=\s+\w+\.json\(\);\n?`).ReplaceAllString(s, "")

		return s, true
	}

	// Complex case: multiple fetches → generate /api/<page> handler comment
	// Add loader pointing to a server-side API endpoint
	funcIdx := strings.Index(s, "export default function")
	pageName := "data"
	nameRe := regexp.MustCompile(`export default function (\w+)`)
	if m := nameRe.FindStringSubmatch(s); m != nil {
		// Convert PricingPage → pricing, AboutPage → about
		name := strings.TrimSuffix(m[1], "Page")
		pageName = strings.ToLower(name)
	}

	loaderLine := fmt.Sprintf("\nexport const loader = { url: \"/api/%s\" };\n", pageName)
	comment := fmt.Sprintf(`// TODO: Add Go API handler in main.go:
// app.API("/api/%s", func(w http.ResponseWriter, r *http.Request) {
//   // Fetch and aggregate data from:
`, pageName)
	for _, url := range fetchURLs {
		comment += fmt.Sprintf("//   //   %s\n", url)
	}
	comment += "//   // Return combined JSON response\n// })\n"

	if funcIdx > 0 {
		s = s[:funcIdx] + comment + loaderLine + "\n" + s[funcIdx:]
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

// detectFavicon checks for common favicon files in the static output.
func (c *converter) detectFavicon() {
	staticDir := filepath.Join(c.dst, "static")
	for _, name := range []string{"favicon.ico", "favicon.png", "favicon.svg", "icon.png", "icon.ico"} {
		path := filepath.Join(staticDir, name)
		if _, err := os.Stat(path); err == nil {
			c.favicon = "/static/" + name
			fmt.Printf("  Favicon: %s\n", c.favicon)
			return
		}
	}
	// Also check layout metadata for icons
	layoutPath := filepath.Join(c.dst, "app", "layout.jsx")
	if data, err := os.ReadFile(layoutPath); err == nil {
		content := string(data)
		if idx := strings.Index(content, `icon: "`); idx >= 0 {
			rest := content[idx+7:]
			if end := strings.Index(rest, `"`); end >= 0 {
				iconPath := rest[:end]
				// Convert /favicon.png → /static/favicon.png
				if strings.HasPrefix(iconPath, "/") {
					c.favicon = "/static" + iconPath
					fmt.Printf("  Favicon (from metadata): %s\n", c.favicon)
				}
			}
		}
	}
}

// extractSiteTitle reads the layout's metadata export to get the site title.
func (c *converter) extractSiteTitle() {
	layoutPath := filepath.Join(c.dst, "app", "layout.jsx")
	data, err := os.ReadFile(layoutPath)
	if err != nil {
		return
	}
	content := string(data)
	idx := strings.Index(content, "export const metadata")
	if idx < 0 {
		return
	}
	// Find title: "..."
	rest := content[idx:]
	titleIdx := strings.Index(rest, `title:`)
	if titleIdx < 0 {
		return
	}
	after := rest[titleIdx+6:]
	q := strings.Index(after, `"`)
	if q < 0 {
		return
	}
	after = after[q+1:]
	q2 := strings.Index(after, `"`)
	if q2 < 0 {
		return
	}
	c.title = after[:q2]
	fmt.Printf("  Title: %s\n", c.title)
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
	// Filter out dev/Next.js-specific vars
	skip := map[string]bool{"LUNGO_DEV": true, "NODE_ENV": true}

	var sb strings.Builder
	for k, v := range c.envVars {
		if skip[k] {
			continue
		}
		sb.WriteString(k + "=" + v + "\n")
	}
	os.WriteFile(filepath.Join(c.dst, ".env"), []byte(sb.String()), 0644)

	var example strings.Builder
	for k := range c.envVars {
		if skip[k] {
			continue
		}
		example.WriteString(k + "=\n")
	}
	os.WriteFile(filepath.Join(c.dst, ".env.example"), []byte(example.String()), 0644)

	// Generate .env.production (same as .env, used by deploy-frontend script)
	os.WriteFile(filepath.Join(c.dst, ".env.production"), []byte(sb.String()), 0644)

	fmt.Println("\n  ENV: generated .env, .env.production, and .env.example")
}

// ── main.go generation ───────────────────────────────

func (c *converter) generateMainGo() {
	var sb strings.Builder
	needsFmt := c.hasAnalytics || c.hasFBPixel
	needsStrings := len(c.envVars) > 0

	sb.WriteString("package main\n\nimport (\n")
	if needsFmt {
		sb.WriteString("\t\"fmt\"\n")
	}
	sb.WriteString("\t\"log\"\n\t\"os\"\n")
	if needsStrings {
		sb.WriteString("\t\"strings\"\n")
	}
	sb.WriteString("\n\t\"github.com/marcoschwartz/lungo/pkg/lungo\"\n)\n\n")

	// Add tracking script builder if needed
	if c.hasAnalytics || c.hasFBPixel {
		sb.WriteString(`func buildTrackingScripts() string {
	var s string
`)
		if c.hasAnalytics {
			sb.WriteString(`	analyticsURL := os.Getenv("ANALYTICS_URL")
	projectID := os.Getenv("OMNIKIT_PROJECT_ID")
	apiKey := os.Getenv("OMNIKIT_API_KEY")
	if analyticsURL != "" && projectID != "" && apiKey != "" {
		s += fmt.Sprintf(` + "`" + `
<script src="%s" defer></script>
<script>window.omnkitQueue=window.omnkitQueue||[];function omnikit(){omnkitQueue.push(arguments);}omnikit('init','%s','%s');</script>` + "`" + `,
			analyticsURL, projectID, apiKey)
	}
`)
		}
		if c.hasFBPixel {
			sb.WriteString(`	pixelID := os.Getenv("FB_PIXEL_ID")
	if pixelID != "" {
		s += fmt.Sprintf(` + "`" + `
<script>!function(f,b,e,v,n,t,s){if(f.fbq)return;n=f.fbq=function(){n.callMethod?n.callMethod.apply(n,arguments):n.queue.push(arguments)};if(!f._fbq)f._fbq=n;n.push=n;n.loaded=!0;n.version='2.0';n.queue=[];t=b.createElement(e);t.async=!0;t.src=v;s=b.getElementsByTagName(e)[0];s.parentNode.insertBefore(t,s)}(window,document,'script','https://connect.facebook.net/en_US/fbevents.js');fbq('init','%s');fbq('track','PageView');</script>` + "`" + `, pixelID)
	}
`)
		}
		sb.WriteString("\treturn s\n}\n\n")
	}

	// Add env preloader if we have env vars
	if len(c.envVars) > 0 {
		sb.WriteString(`func loadEnv() {
	data, err := os.ReadFile(".env")
	if err != nil { return }
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line[0] == '#' { continue }
		if eq := strings.IndexByte(line, '='); eq > 0 {
			if os.Getenv(line[:eq]) == "" { os.Setenv(line[:eq], line[eq+1:]) }
		}
	}
}

`)
	}

	sb.WriteString("func main() {\n")
	if len(c.envVars) > 0 {
		sb.WriteString("\tloadEnv()\n")
	}
	sb.WriteString(`	dev := os.Getenv("LUNGO_DEV") == "1"

	app := lungo.New(lungo.Options{
		AppDir:       "./app",
		StaticDir:    "./static",
		Dev:          dev,
		DefaultTheme: "dark",`)

	// Add HeadExtra with favicon and tracking scripts
	hasHeadExtra := c.favicon != "" || c.hasAnalytics || c.hasFBPixel
	if hasHeadExtra {
		if c.hasAnalytics || c.hasFBPixel {
			// Use buildTrackingScripts() function for dynamic env-based scripts
			faviconPart := ""
			if c.favicon != "" {
				faviconPart = fmt.Sprintf(`"<link rel=\"icon\" href=\"%s\">" + `, c.favicon)
			}
			sb.WriteString(fmt.Sprintf("\n\t\tHeadExtra: %sbuildTrackingScripts(),", faviconPart))
		} else if c.favicon != "" {
			sb.WriteString(fmt.Sprintf("\n\t\tHeadExtra:    \"<link rel=\\\"icon\\\" href=\\\"%s\\\">\",", c.favicon))
		}
	}

	sb.WriteString(`
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

require github.com/marcoschwartz/lungo v1.1.0

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

func (c *converter) generateDockerfile() {
	hasEnv := len(c.envVars) > 0
	copyEnv := ""
	if hasEnv {
		copyEnv = "\nCOPY --from=builder /build/.env /app/.env"
	}

	content := fmt.Sprintf(`# Build
FROM golang:1.25-alpine AS builder
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /server .
RUN apk add --no-cache upx && upx --best /server

# Run — scratch: no OS, just the binary (~7MB)
FROM scratch
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /server /server
COPY --from=builder /build/app/ /app/
COPY --from=builder /build/static/ /static/%s
EXPOSE 3000
ENTRYPOINT ["/server"]
`, copyEnv)

	os.WriteFile(filepath.Join(c.dst, "Dockerfile"), []byte(content), 0644)
	fmt.Println("  Generated Dockerfile")
}

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
