package lungo

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// BuildOptions configures static site generation.
type BuildOptions struct {
	// OutputDir is where the static files will be written. Defaults to "out".
	OutputDir string
}

// Build generates a fully static site from the app.
// It renders every route to HTML, copies static assets and JS files,
// and writes everything to the output directory.
//
// Usage:
//
//	app := reactgo.New(reactgo.Options{AppDir: "./app", StaticDir: "./static"})
//	app.API("/api/hello", helloHandler)
//	app.Build(reactgo.BuildOptions{OutputDir: "./out"})
func (a *App) Build(opts BuildOptions) error {
	if opts.OutputDir == "" {
		opts.OutputDir = "out"
	}

	absOut, err := filepath.Abs(opts.OutputDir)
	if err != nil {
		return fmt.Errorf("invalid output dir: %w", err)
	}

	// Clean output dir
	os.RemoveAll(absOut)

	fmt.Println("Building static site...")

	// 1. Render all pages to HTML
	routes := a.router.Routes()
	for _, pattern := range routes {
		route := a.router.Match(patternToExample(pattern))
		if route == nil {
			continue
		}

		// Fetch loader data
		var loaderData json.RawMessage
		if route.HasLoader && route.LoaderURL != "" {
			fakeReq, _ := http.NewRequest("GET", route.LoaderURL, nil)
			loaderData = a.fetchLoaderData(route, fakeReq)
		}

		html := a.renderPage(route, loaderData)

		// Determine output path
		outPath := filepath.Join(absOut, pattern)
		if pattern == "/" {
			outPath = filepath.Join(absOut)
		}
		outFile := filepath.Join(outPath, "index.html")

		if err := os.MkdirAll(filepath.Dir(outFile), 0755); err != nil {
			return fmt.Errorf("mkdir %s: %w", filepath.Dir(outFile), err)
		}
		if err := os.WriteFile(outFile, []byte(html), 0644); err != nil {
			return fmt.Errorf("write %s: %w", outFile, err)
		}
		fmt.Printf("  %s → %s\n", pattern, outFile)
	}

	// 2. Write the runtime JS
	runtimeDir := filepath.Join(absOut, "runtime")
	os.MkdirAll(runtimeDir, 0755)
	if err := os.WriteFile(filepath.Join(runtimeDir, "lungo.js"), runtimeJS, 0644); err != nil {
		return fmt.Errorf("write runtime: %w", err)
	}
	fmt.Println("  /runtime/lungo.js")

	// 3. Copy app JS files
	appOutDir := filepath.Join(absOut, "app")
	if err := a.copyAppFiles(appOutDir); err != nil {
		return fmt.Errorf("copy app files: %w", err)
	}

	// 4. Copy static files
	staticOutDir := filepath.Join(absOut, "static")
	if err := a.copyStaticFiles(staticOutDir); err != nil {
		return fmt.Errorf("copy static files: %w", err)
	}

	// 5. Generate loader data JSON files for client-side navigation
	dataDir := filepath.Join(absOut, "_data")
	for _, pattern := range routes {
		route := a.router.Match(patternToExample(pattern))
		if route == nil || !route.HasLoader {
			continue
		}

		fakeReq, _ := http.NewRequest("GET", route.LoaderURL, nil)
		data := a.fetchLoaderData(route, fakeReq)
		if data == nil {
			data = json.RawMessage("{}")
		}

		dataPath := filepath.Join(dataDir, pattern)
		if pattern == "/" {
			dataPath = dataDir
		}
		dataFile := filepath.Join(dataPath, "index.json")
		os.MkdirAll(filepath.Dir(dataFile), 0755)
		os.WriteFile(dataFile, data, 0644)
		fmt.Printf("  /_data%s → %s\n", pattern, dataFile)
	}

	// Count files
	fileCount := 0
	filepath.WalkDir(absOut, func(path string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			fileCount++
		}
		return nil
	})

	fmt.Printf("\nBuild complete: %d files in %s\n", fileCount, absOut)
	return nil
}

func (a *App) copyAppFiles(outDir string) error {
	walkFn := func(relPath string, data []byte) error {
		if !strings.HasSuffix(relPath, ".js") {
			return nil
		}
		outFile := filepath.Join(outDir, relPath)
		if err := os.MkdirAll(filepath.Dir(outFile), 0755); err != nil {
			return err
		}
		fmt.Printf("  /app/%s\n", relPath)
		return os.WriteFile(outFile, data, 0644)
	}

	if a.opts.AppFS != nil {
		return walkFS(a.opts.AppFS, ".", walkFn)
	}
	return walkDisk(a.opts.AppDir, a.opts.AppDir, walkFn)
}

func (a *App) copyStaticFiles(outDir string) error {
	walkFn := func(relPath string, data []byte) error {
		outFile := filepath.Join(outDir, relPath)
		if err := os.MkdirAll(filepath.Dir(outFile), 0755); err != nil {
			return err
		}
		fmt.Printf("  /static/%s\n", relPath)
		return os.WriteFile(outFile, data, 0644)
	}

	if a.opts.StaticFS != nil {
		return walkFS(a.opts.StaticFS, ".", walkFn)
	}
	return walkDisk(a.opts.StaticDir, a.opts.StaticDir, walkFn)
}

func walkFS(fsys fs.FS, root string, fn func(string, []byte) error) error {
	return fs.WalkDir(fsys, root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		data, err := fs.ReadFile(fsys, path)
		if err != nil {
			return err
		}
		return fn(path, data)
	})
}

func walkDisk(dir, base string, fn func(string, []byte) error) error {
	return filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(base, path)
		rel = filepath.ToSlash(rel)
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return fn(rel, data)
	})
}
