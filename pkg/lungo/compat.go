package lungo

import (
	"regexp"
	"strings"
)

// NextCompat transforms Next.js / React conventions into our framework's format.
// This runs BEFORE the JSX transpiler, converting imports, directives, attributes,
// and component names so that Next.js pages work without modification.
func NextCompat(source string) string {
	s := source

	// 1. Strip "use client" / "use server" directives
	s = stripDirectives(s)

	// 2. Convert React imports → window.Lungo destructuring
	s = convertReactImports(s)

	// 3. Strip next/* imports and convert components
	s = convertNextImports(s)

	// 4. Strip TypeScript: type annotations, interfaces, generics
	s = stripTypeScript(s)

	// 5. className → class
	s = convertAttributes(s)

	return s
}

// stripDirectives removes "use client" and "use server" directives.
func stripDirectives(s string) string {
	s = regexp.MustCompile(`(?m)^\s*["']use client["']\s*;?\s*\n?`).ReplaceAllString(s, "")
	s = regexp.MustCompile(`(?m)^\s*["']use server["']\s*;?\s*\n?`).ReplaceAllString(s, "")
	return s
}

// convertReactImports converts React/hooks imports to window.Lungo.
func convertReactImports(s string) string {
	// import { useState, useEffect } from "react"
	// import { useState, useEffect } from 'react'
	// import React, { useState } from "react"
	re := regexp.MustCompile(`import\s+(?:React\s*,?\s*)?\{([^}]+)\}\s+from\s+["']react["']\s*;?`)
	matches := re.FindStringSubmatch(s)
	if matches != nil {
		hooks := strings.TrimSpace(matches[1])
		// Clean up TypeScript type imports: import { useState, type FC } from "react"
		var cleaned []string
		for _, h := range strings.Split(hooks, ",") {
			h = strings.TrimSpace(h)
			if h == "" || strings.HasPrefix(h, "type ") {
				continue
			}
			cleaned = append(cleaned, h)
		}

		// Check if we already have a window.Lungo destructuring
		if !strings.Contains(s, "window.Lungo") {
			replacement := "const { h, " + strings.Join(cleaned, ", ") + " } = window.Lungo;"
			// Make sure h is not duplicated
			replacement = strings.Replace(replacement, "h, h,", "h,", 1)
			replacement = strings.Replace(replacement, "h, h ", "h ", 1)
			s = re.ReplaceAllString(s, replacement)
		} else {
			s = re.ReplaceAllString(s, "")
		}
	}

	// import React from "react" (default import only)
	reDefault := regexp.MustCompile(`import\s+React\s+from\s+["']react["']\s*;?\n?`)
	s = reDefault.ReplaceAllString(s, "")

	// import * as React from "react"
	reStar := regexp.MustCompile(`import\s+\*\s+as\s+React\s+from\s+["']react["']\s*;?\n?`)
	s = reStar.ReplaceAllString(s, "")

	// React.useState → useState (if we've already destructured)
	s = strings.ReplaceAll(s, "React.useState", "useState")
	s = strings.ReplaceAll(s, "React.useEffect", "useEffect")
	s = strings.ReplaceAll(s, "React.useRef", "useRef")
	s = strings.ReplaceAll(s, "React.useMemo", "useMemo")
	s = strings.ReplaceAll(s, "React.useCallback", "useCallback")

	return s
}

// convertNextImports strips next/* imports and converts their usages.
func convertNextImports(s string) string {
	// import Link from "next/link"
	reLinkImport := regexp.MustCompile(`import\s+(\w+)\s+from\s+["']next/link["']\s*;?\n?`)
	linkMatch := reLinkImport.FindStringSubmatch(s)
	linkName := "Link"
	if linkMatch != nil {
		linkName = linkMatch[1]
		s = reLinkImport.ReplaceAllString(s, "")
	}
	// <Link href="..."> → <a href="...">
	// <Link ...> → <a ...>
	if linkMatch != nil {
		s = regexp.MustCompile(`<`+linkName+`(\s)`).ReplaceAllString(s, "<a$1")
		s = regexp.MustCompile(`<`+linkName+`>`).ReplaceAllString(s, "<a>")
		s = regexp.MustCompile(`</`+linkName+`>`).ReplaceAllString(s, "</a>")
	}

	// import Image from "next/image"
	reImageImport := regexp.MustCompile(`import\s+(\w+)\s+from\s+["']next/image["']\s*;?\n?`)
	imageMatch := reImageImport.FindStringSubmatch(s)
	imageName := "Image"
	if imageMatch != nil {
		imageName = imageMatch[1]
		s = reImageImport.ReplaceAllString(s, "")
	}
	// <Image ... /> → <img ... />
	if imageMatch != nil {
		s = regexp.MustCompile(`<`+imageName+`(\s)`).ReplaceAllString(s, "<img$1")
		s = regexp.MustCompile(`</`+imageName+`>`).ReplaceAllString(s, "</img>")
		// Remove Image-specific props: priority, quality, placeholder, blurDataURL
		s = regexp.MustCompile(`\s+priority(\s|/|>)`).ReplaceAllString(s, "$1")
		s = regexp.MustCompile(`\s+quality=\{[^}]*\}`).ReplaceAllString(s, "")
		s = regexp.MustCompile(`\s+placeholder="[^"]*"`).ReplaceAllString(s, "")
		s = regexp.MustCompile(`\s+blurDataURL="[^"]*"`).ReplaceAllString(s, "")
	}

	// import { useRouter } from "next/navigation"
	reNavRouter := regexp.MustCompile(`import\s+\{([^}]*)\}\s+from\s+["']next/navigation["']\s*;?\n?`)
	navMatch := reNavRouter.FindStringSubmatch(s)
	if navMatch != nil {
		s = reNavRouter.ReplaceAllString(s, "")
		hooks := navMatch[1]

		// usePathname() → useRouter().pathname
		if strings.Contains(hooks, "usePathname") {
			s = strings.ReplaceAll(s, "usePathname()", "useRouter().pathname")
		}

		// useSearchParams() → new URLSearchParams(useRouter().query)
		if strings.Contains(hooks, "useSearchParams") {
			s = strings.ReplaceAll(s, "useSearchParams()", "new URLSearchParams(useRouter().query)")
		}

		// useParams() → window.__LUNGO_ROUTE__?.params
		if strings.Contains(hooks, "useParams") {
			s = strings.ReplaceAll(s, "useParams()", "(window.__LUNGO_ROUTE__?.params || {})")
		}
	}

	// import { useRouter } from "next/router" (pages router)
	rePagesRouter := regexp.MustCompile(`import\s+\{[^}]*\}\s+from\s+["']next/router["']\s*;?\n?`)
	s = rePagesRouter.ReplaceAllString(s, "")

	// import Head from "next/head" → strip (we use metadata export)
	reHead := regexp.MustCompile(`import\s+\w+\s+from\s+["']next/head["']\s*;?\n?`)
	s = reHead.ReplaceAllString(s, "")

	// import type ... from ... → strip all type-only imports
	reTypeImport := regexp.MustCompile(`import\s+type\s+[^;]+;?\n?`)
	s = reTypeImport.ReplaceAllString(s, "")

	// import { type Foo, ... } — handled in convertReactImports

	// Strip any remaining next/* imports
	reNextAny := regexp.MustCompile(`import\s+[^;]+from\s+["']next/[^"']+["']\s*;?\n?`)
	s = reNextAny.ReplaceAllString(s, "")

	return s
}

// stripTypeScript removes TypeScript syntax that's not valid JS.
func stripTypeScript(s string) string {
	// interface Foo { ... }
	s = regexp.MustCompile(`(?m)^(?:export\s+)?interface\s+\w+\s*(?:extends\s+[^{]+)?\{[^}]*\}\s*\n?`).ReplaceAllString(s, "")

	// type Foo = ...;
	s = regexp.MustCompile(`(?m)^(?:export\s+)?type\s+\w+\s*=[^;]+;\s*\n?`).ReplaceAllString(s, "")

	// Function parameter types: (foo: string, bar: number) → (foo, bar)
	// Also handles custom types: ({ title }: Props) → ({ title })
	s = regexp.MustCompile(`:\s*(?:string|number|boolean|any|void|never)\b`).ReplaceAllString(s, "")
	// Custom type annotations after } or identifier before ) or ,
	s = regexp.MustCompile(`\}:\s*[A-Z]\w*(?:<[^>]*>)?\s*\)`).ReplaceAllStringFunc(s, func(m string) string {
		return strings.Replace(m[:strings.Index(m, ":")], "", "", 0) + ")"
	})
	s = regexp.MustCompile(`(\w):\s*[A-Z]\w*(?:<[^>]*>)?(\s*[,)])`).ReplaceAllString(s, "$1$2")

	// Generic type params on functions: function foo<T>(...)  → function foo(...)
	s = regexp.MustCompile(`(<\w+(?:\s*,\s*\w+)*(?:\s+extends\s+[^>]+)?>)(\s*\()`).ReplaceAllString(s, "$2")

	// Type assertions: foo as Bar → foo (only when followed by uppercase type name)
	s = regexp.MustCompile(`\s+as\s+[A-Z]\w*(?:\[\])?`).ReplaceAllString(s, "")

	// React.FC<Props>, React.ReactNode etc in type positions
	// }: { children: React.ReactNode }) → })
	s = regexp.MustCompile(`:\s*React\.\w+(?:<[^>]*>)?`).ReplaceAllString(s, "")

	// Readonly<{ ... }> wrapper in params — simplify to just { ... }
	s = regexp.MustCompile(`Readonly<(\{[^}]*\})>`).ReplaceAllString(s, "$1")

	// Remove ?: optional marker (: already stripped above leaves just ?)
	// Actually leave ? alone — it's valid JS in ternaries

	return s
}

// convertAttributes converts React attribute conventions to HTML.
func convertAttributes(s string) string {
	// className → class (but not inside strings or comments)
	s = convertAttrOutsideStrings(s, "className", "class")

	// React event handlers → lowercase HTML events
	eventMap := map[string]string{
		"onClick":     "onclick",
		"onChange":     "onchange",
		"onSubmit":     "onsubmit",
		"onInput":      "oninput",
		"onKeyDown":    "onkeydown",
		"onKeyUp":      "onkeyup",
		"onKeyPress":   "onkeypress",
		"onMouseDown":  "onmousedown",
		"onMouseUp":    "onmouseup",
		"onMouseEnter": "onmouseenter",
		"onMouseLeave": "onmouseleave",
		"onMouseMove":  "onmousemove",
		"onFocus":      "onfocus",
		"onBlur":       "onblur",
		"onDragStart":  "ondragstart",
		"onDragEnd":    "ondragend",
		"onDragOver":   "ondragover",
		"onDrop":       "ondrop",
		"onScroll":     "onscroll",
		"onTouchStart": "ontouchstart",
		"onTouchEnd":   "ontouchend",
		"onTouchMove":  "ontouchmove",
	}
	for react, html := range eventMap {
		s = convertAttrOutsideStrings(s, react, html)
	}

	// htmlFor → for
	s = convertAttrOutsideStrings(s, "htmlFor", "for")

	return s
}

// convertAttrOutsideStrings replaces attribute/event handler names when they appear
// as JSX attributes, destructured props, or variable references — not inside strings.
func convertAttrOutsideStrings(s, from, to string) string {
	// For reserved words like "class" and "for", only rename when used as a JSX attribute
	// (followed by = or : ), not in destructuring ({ className }) or variable references.
	if to == "class" || to == "for" {
		// Only match: className= or className: (JSX attribute assignment)
		re := regexp.MustCompile(from + `([\s]*=)`)
		s = re.ReplaceAllString(s, to+"${1}")
		return s
	}
	// For non-reserved names, rename in both attributes and destructuring
	re := regexp.MustCompile(`(\s)` + from + `([\s=>])`)
	s = re.ReplaceAllString(s, "${1}"+to+"${2}")
	re2 := regexp.MustCompile(`([{,(])(\s*)` + from + `(\s*[,})])`)
	s = re2.ReplaceAllString(s, "${1}${2}"+to+"${3}")
	return s
}
