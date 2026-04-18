package lungo

import (
	"fmt"
	"path"
	"regexp"
	"strings"

	"github.com/marcoschwartz/espresso"
)

// SSR module system.
//
// Browsers already resolve ES imports at runtime when RouterView imports a
// page module, so pages can freely `import { Hero } from "/app/blocks.js"`
// on the client. The server-side espresso evaluator, however, sees one
// module in isolation — it doesn't follow imports. That meant shared
// components (e.g. a block library) had to be inlined in every page just to
// render correctly on the server, defeating the point of a shared file.
//
// This file adds a minimal module loader for SSR:
//   - parse supported import statements and strip them from source
//   - resolve specifiers relative to AppDir
//   - recursively load + cache each imported module's top-level bindings
//   - merge imported names into the importing module's evaluation scope
//
// Scoping model is flat-namespace merge, not true ES-module closures —
// imported functions see names from the importer, not from their original
// module. In practice this is rare to observe because each module's helpers
// are loaded alongside its exports.

// Supported forms (one per line):
//   import { A, B as C } from "/path";
//   import "/path";
// Not supported: default imports, `* as X`, TypeScript `import type`.
var (
	namedImportRE   = regexp.MustCompile(`(?m)^\s*import\s*\{\s*([^}]+?)\s*\}\s*from\s*["']([^"']+)["']\s*;?\s*$`)
	bareImportRE    = regexp.MustCompile(`(?m)^\s*import\s*["']([^"']+)["']\s*;?\s*$`)
	exportKeywordRE = regexp.MustCompile(`(?m)^(\s*)export\s+(const|let|var|function|async\s+function|class)\b`)
)

// importSpec is a parsed single import statement.
type importSpec struct {
	// names maps local binding → exported name. For `import { A, B as C }`
	// this is {"A":"A", "C":"B"}. Empty for side-effect imports — in that
	// case we still load the module so its transitive scope is realized.
	names map[string]string
	path  string
}

// parseImports extracts supported import statements and returns the source
// with those statements removed. Lines are replaced with empty strings so
// line numbers stay stable for error messages.
func parseImports(source string) ([]importSpec, string) {
	var specs []importSpec

	cleaned := namedImportRE.ReplaceAllStringFunc(source, func(m string) string {
		sub := namedImportRE.FindStringSubmatch(m)
		if len(sub) != 3 {
			return m
		}
		names := map[string]string{}
		for _, part := range strings.Split(sub[1], ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			if idx := strings.Index(part, " as "); idx >= 0 {
				imported := strings.TrimSpace(part[:idx])
				local := strings.TrimSpace(part[idx+4:])
				names[local] = imported
			} else {
				names[part] = part
			}
		}
		specs = append(specs, importSpec{names: names, path: sub[2]})
		return ""
	})

	cleaned = bareImportRE.ReplaceAllStringFunc(cleaned, func(m string) string {
		sub := bareImportRE.FindStringSubmatch(m)
		if len(sub) != 2 {
			return m
		}
		specs = append(specs, importSpec{path: sub[1]})
		return ""
	})

	return specs, cleaned
}

// stripExportKeyword strips `export` before named declarations so espresso's
// ExtractFunctions/ExtractTopLevelVars see them as plain declarations.
// Leaves `export default ...` alone — ExtractDefaultExport handles that.
func stripExportKeyword(source string) string {
	return exportKeywordRE.ReplaceAllString(source, "$1$2")
}

// resolveImportPath turns an import specifier into an AppDir-relative path.
//   /app/foo.js → foo.js
//   ./bar       → <dir-of-importer>/bar
//   bar         → <dir-of-importer>/bar
func resolveImportPath(spec, importerRel string) string {
	if strings.HasPrefix(spec, "/app/") {
		return strings.TrimPrefix(spec, "/app/")
	}
	base := path.Dir(importerRel)
	if base == "." {
		base = ""
	}
	return path.Clean(path.Join(base, spec))
}

// moduleCacheEntry caches an imported module's top-level scope. Prod-only;
// dev always re-reads so HMR on blocks files works.
type moduleCacheEntry struct {
	names map[string]*espresso.Value
}

// getModuleScope loads a module and returns its top-level bindings (funcs +
// vars, transitively including anything it imports) as espresso values.
// Handles .js → .jsx fallback, missing-extension lookup, and import cycles.
func (a *App) getModuleScope(modulePath string, stack []string) (map[string]*espresso.Value, error) {
	for _, s := range stack {
		if s == modulePath {
			return nil, fmt.Errorf("import cycle: %s -> %s", strings.Join(stack, " -> "), modulePath)
		}
	}

	if !a.opts.Dev {
		a.moduleCacheMu.RLock()
		cached, ok := a.moduleCache[modulePath]
		a.moduleCacheMu.RUnlock()
		if ok {
			return cached.names, nil
		}
	}

	data, resolved, err := a.loadModuleFile(modulePath)
	if err != nil {
		return nil, fmt.Errorf("module not found: %s", modulePath)
	}
	source := string(data)

	if strings.HasSuffix(resolved, ".jsx") {
		source, _ = TranspileJSXWithErrors(source)
	}
	source = stripExportKeyword(source)

	specs, cleaned := parseImports(source)

	// Seed the module's scope with SSR runtime stubs (h, hooks, Image, …) so
	// JSX inside this module resolves the same way a page does. Critically,
	// this gives the module its own h — when a wrapped function from this
	// module runs, its h(...) calls resolve against *this* module's scope
	// rather than the caller's, which is what makes `const registry = { Hero }`
	// referenced from `BlockRenderer` work transparently.
	scope := buildSSRScope(nil)

	newStack := append(append([]string(nil), stack...), modulePath)
	for _, spec := range specs {
		childPath := resolveImportPath(spec.path, modulePath)
		child, err := a.getModuleScope(childPath, newStack)
		if err != nil {
			continue
		}
		if len(spec.names) == 0 {
			for k, v := range child {
				scope[k] = wrapImport(v, child)
			}
			continue
		}
		for local, imported := range spec.names {
			if v, ok := child[imported]; ok {
				scope[local] = wrapImport(v, child)
			}
		}
	}

	for name, fn := range espresso.ExtractFunctions(cleaned) {
		scope[name] = fn
	}
	// Evaluate top-level vars directly into `scope` so declarations like
	// `const registry = { Hero, FeatureGrid }` can see already-extracted
	// functions. Passing an empty map here would make those refs undefined.
	espresso.ExtractTopLevelVars(cleaned, scope)

	if !a.opts.Dev {
		a.moduleCacheMu.Lock()
		a.moduleCache[modulePath] = moduleCacheEntry{names: scope}
		a.moduleCacheMu.Unlock()
	}

	return scope, nil
}

// wrapImport binds an exported value to its defining module's scope. If the
// value is a function, we return a native wrapper that, when invoked, calls
// the original function with the module scope — preserving ES-module
// semantics: an imported `BlockRenderer` that references `registry` from its
// own module continues to see that `registry`, not whatever happens to be in
// the caller's scope.
func wrapImport(v *espresso.Value, moduleScope map[string]*espresso.Value) *espresso.Value {
	if v == nil || v.Type() != espresso.TypeFunc {
		return v
	}
	return espresso.NewNativeFunc(func(args []*espresso.Value) *espresso.Value {
		// h() passes props as a single object; translate to the name→value
		// shape CallFunc expects.
		propsObj := map[string]*espresso.Value{}
		if len(args) > 0 && args[0] != nil && args[0].Type() == espresso.TypeObject && args[0].Object() != nil {
			for k, val := range args[0].Object() {
				propsObj[k] = val
			}
		}
		return espresso.CallFunc(moduleScope, v, propsObj)
	})
}

// loadModuleFile reads a module's bytes, trying .jsx fallback and appending
// .js/.jsx when the specifier has no extension.
func (a *App) loadModuleFile(modulePath string) ([]byte, string, error) {
	tryPaths := []string{modulePath}
	if !strings.HasSuffix(modulePath, ".js") && !strings.HasSuffix(modulePath, ".jsx") {
		tryPaths = []string{modulePath + ".js", modulePath + ".jsx", modulePath}
	}
	for _, p := range tryPaths {
		if data, err := a.readAppFile(p); err == nil {
			resolved := p
			if strings.HasSuffix(p, ".js") && !a.hasAppFile(p) {
				resolved = strings.TrimSuffix(p, ".js") + ".jsx"
			}
			return data, resolved, nil
		}
	}
	return nil, "", fmt.Errorf("not found")
}
