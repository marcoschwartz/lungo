package lungo

import (
	"strings"
	"testing"
)

// ─── Directive stripping ────────────────────────────────────────

func TestStripUseClient(t *testing.T) {
	input := `"use client";
import { useState } from "react";
export default function Page() {}`
	out := NextCompat(input)
	if strings.Contains(out, "use client") {
		t.Error("should strip 'use client'")
	}
	if !strings.Contains(out, "function Page") {
		t.Error("should keep the component")
	}
}

func TestStripUseServer(t *testing.T) {
	input := `'use server'
export async function action() {}`
	out := NextCompat(input)
	if strings.Contains(out, "use server") {
		t.Error("should strip 'use server'")
	}
}

// ─── React imports ──────────────────────────────────────────────

func TestConvertReactImport(t *testing.T) {
	input := `import { useState, useEffect } from "react";`
	out := NextCompat(input)
	if !strings.Contains(out, "window.Lungo") {
		t.Errorf("should convert to window.Lungo, got: %s", out)
	}
	if !strings.Contains(out, "useState") || !strings.Contains(out, "useEffect") {
		t.Error("should keep hook names")
	}
	if !strings.Contains(out, "{ h,") {
		t.Errorf("should add h to destructuring, got: %s", out)
	}
}

func TestConvertReactDefaultImport(t *testing.T) {
	input := `import React from "react";
const x = React.useState(0);`
	out := NextCompat(input)
	if strings.Contains(out, `import React`) {
		t.Error("should strip default React import")
	}
	if !strings.Contains(out, "useState(0)") {
		t.Error("should convert React.useState to useState")
	}
}

func TestConvertReactStarImport(t *testing.T) {
	input := `import * as React from "react";`
	out := NextCompat(input)
	if strings.Contains(out, "import") {
		t.Error("should strip star import")
	}
}

func TestConvertReactImportWithTypes(t *testing.T) {
	input := `import { useState, type FC, useEffect } from "react";`
	out := NextCompat(input)
	if strings.Contains(out, "FC") {
		t.Error("should strip type imports")
	}
	if !strings.Contains(out, "useState") || !strings.Contains(out, "useEffect") {
		t.Error("should keep non-type imports")
	}
}

// ─── Next.js imports ────────────────────────────────────────────

func TestConvertNextLink(t *testing.T) {
	input := `import Link from "next/link";
export default function Page() {
  return <Link href="/about">About</Link>;
}`
	out := NextCompat(input)
	if strings.Contains(out, "next/link") {
		t.Error("should strip next/link import")
	}
	if !strings.Contains(out, "<a href") {
		t.Errorf("should convert Link to a, got: %s", out)
	}
	if !strings.Contains(out, "</a>") {
		t.Error("should convert closing </Link> to </a>")
	}
}

func TestConvertNextImage(t *testing.T) {
	input := `import Image from "next/image";
export default function Page() {
  return <Image src="/photo.jpg" width={100} height={100} priority />;
}`
	out := NextCompat(input)
	if strings.Contains(out, "next/image") {
		t.Error("should strip next/image import")
	}
	if !strings.Contains(out, "<img ") {
		t.Errorf("should convert Image to img, got: %s", out)
	}
	if strings.Contains(out, "priority") {
		t.Error("should strip priority prop")
	}
}

func TestConvertNextNavigation(t *testing.T) {
	input := `import { useRouter, usePathname, useSearchParams, useParams } from "next/navigation";
const path = usePathname();
const params = useSearchParams();
const p = useParams();`
	out := NextCompat(input)
	if strings.Contains(out, "next/navigation") {
		t.Error("should strip next/navigation import")
	}
	if !strings.Contains(out, "useRouter().pathname") {
		t.Errorf("should convert usePathname, got: %s", out)
	}
	if !strings.Contains(out, "URLSearchParams(useRouter().query)") {
		t.Error("should convert useSearchParams")
	}
	if !strings.Contains(out, "__LUNGO_ROUTE__") {
		t.Error("should convert useParams")
	}
}

func TestStripNextHead(t *testing.T) {
	input := `import Head from "next/head";`
	out := NextCompat(input)
	if strings.Contains(out, "import") {
		t.Error("should strip next/head import")
	}
}

func TestStripTypeImport(t *testing.T) {
	input := `import type { Metadata } from "next";`
	out := NextCompat(input)
	if strings.Contains(out, "import") {
		t.Error("should strip type-only imports")
	}
}

// ─── TypeScript stripping ───────────────────────────────────────

func TestStripInterface(t *testing.T) {
	input := `interface Props {
  name: string;
  age: number;
}
function Foo() {}`
	out := NextCompat(input)
	if strings.Contains(out, "interface") {
		t.Error("should strip interface")
	}
	if !strings.Contains(out, "function Foo") {
		t.Error("should keep function")
	}
}

func TestStripTypeAlias(t *testing.T) {
	input := `type Theme = "light" | "dark";
const x = 1;`
	out := NextCompat(input)
	if strings.Contains(out, "type Theme") {
		t.Error("should strip type alias")
	}
	if !strings.Contains(out, "const x = 1") {
		t.Error("should keep code")
	}
}

func TestStripParamTypes(t *testing.T) {
	input := `function greet(name: string, count: number) {}`
	out := NextCompat(input)
	if strings.Contains(out, ": string") || strings.Contains(out, ": number") {
		t.Errorf("should strip param types, got: %s", out)
	}
}

func TestStripAssertion(t *testing.T) {
	input := `const x = data as Post[];`
	out := NextCompat(input)
	if strings.Contains(out, " as ") {
		t.Errorf("should strip type assertion, got: %s", out)
	}
}

// ─── Attribute conversion ───────────────────────────────────────

func TestConvertClassName(t *testing.T) {
	input := `<div className="foo">bar</div>`
	out := NextCompat(input)
	if strings.Contains(out, "className") {
		t.Errorf("should convert className to class, got: %s", out)
	}
	if !strings.Contains(out, " class=") {
		t.Error("should have class attribute")
	}
}

func TestConvertOnClick(t *testing.T) {
	input := `<button onClick={handler}>click</button>`
	out := NextCompat(input)
	if strings.Contains(out, "onClick") {
		t.Errorf("should convert onClick to onclick, got: %s", out)
	}
	if !strings.Contains(out, " onclick=") {
		t.Error("should have onclick attribute")
	}
}

func TestConvertOnChange(t *testing.T) {
	input := `<input onChange={handler} />`
	out := NextCompat(input)
	if !strings.Contains(out, " onchange=") {
		t.Errorf("should convert onChange to onchange, got: %s", out)
	}
}

func TestConvertHtmlFor(t *testing.T) {
	input := `<label htmlFor="email">Email</label>`
	out := NextCompat(input)
	if !strings.Contains(out, " for=") {
		t.Errorf("should convert htmlFor to for, got: %s", out)
	}
}

func TestConvertMultipleEvents(t *testing.T) {
	input := `<div onMouseEnter={enter} onMouseLeave={leave} onKeyDown={key}>test</div>`
	out := NextCompat(input)
	if !strings.Contains(out, "onmouseenter") || !strings.Contains(out, "onmouseleave") || !strings.Contains(out, "onkeydown") {
		t.Errorf("should convert all events, got: %s", out)
	}
}

// ─── Full page conversion ───────────────────────────────────────

func TestFullNextJSPage(t *testing.T) {
	input := `"use client";
import { useState, useEffect, type FC } from "react";
import Link from "next/link";
import Image from "next/image";
import type { Metadata } from "next";

interface Props {
  title: string;
}

export default function Page({ title }: Props) {
  const [count, setCount] = useState(0);

  return (
    <div className="container">
      <h1 onClick={() => setCount(count + 1)}>{title}</h1>
      <p>Count: {count}</p>
      <Link href="/about">About</Link>
      <Image src="/photo.jpg" width={200} height={200} priority />
    </div>
  );
}`
	out := NextCompat(input)

	// Should NOT contain
	checks := []struct {
		shouldNotContain string
		desc             string
	}{
		{`"use client"`, "use client directive"},
		{`from "react"`, "react import"},
		{`from "next/link"`, "next/link import"},
		{`from "next/image"`, "next/image import"},
		{`from "next"`, "next type import"},
		{"interface Props", "TypeScript interface"},
		{": Props", "type annotation"},
		{"className", "className (should be class)"},
		{"onClick", "onClick (should be onclick)"},
		{"<Link ", "Link component"},
		{"<Image ", "Image component"},
		{"priority", "priority prop"},
	}
	for _, c := range checks {
		if strings.Contains(out, c.shouldNotContain) {
			t.Errorf("should not contain %s (%s)\nGot:\n%s", c.shouldNotContain, c.desc, out)
		}
	}

	// Should contain
	musts := []struct {
		shouldContain string
		desc          string
	}{
		{"window.Lungo", "Lungo destructuring"},
		{"useState", "useState hook"},
		{"useEffect", "useEffect hook"},
		{" class=", "class attribute"},
		{" onclick=", "onclick attribute"},
		{"<a href=", "a tag (converted from Link)"},
		{"<img src=", "img tag (converted from Image)"},
		{"function Page", "component function"},
	}
	for _, c := range musts {
		if !strings.Contains(out, c.shouldContain) {
			t.Errorf("should contain %s (%s)\nGot:\n%s", c.shouldContain, c.desc, out)
		}
	}
}

// ─── JSX transpiler tests ───────────────────────────────────────

func TestTranspileSimpleDiv(t *testing.T) {
	input := `function App() { return (<div class="foo">hello</div>); }`
	out := TranspileJSX(input)
	if !strings.Contains(out, `h("div"`) {
		t.Errorf("should transpile div, got: %s", out)
	}
	if !strings.Contains(out, `class: "foo"`) {
		t.Error("should have class prop")
	}
}

func TestTranspileComponent(t *testing.T) {
	input := `function App() { return (<Counter initial={5} />); }`
	out := TranspileJSX(input)
	if !strings.Contains(out, `h(Counter`) {
		t.Errorf("should transpile component, got: %s", out)
	}
}

func TestTranspileSelfClosing(t *testing.T) {
	input := `function App() { return (<br />); }`
	out := TranspileJSX(input)
	if !strings.Contains(out, `h("br"`) {
		t.Errorf("should transpile self-closing, got: %s", out)
	}
}

func TestTranspileNested(t *testing.T) {
	input := `function App() { return (<div><span>hi</span></div>); }`
	out := TranspileJSX(input)
	if !strings.Contains(out, `h("div"`) || !strings.Contains(out, `h("span"`) {
		t.Errorf("should transpile nested elements, got: %s", out)
	}
}

func TestTranspileConditional(t *testing.T) {
	input := `function App() { return (<div>{show && (<span>visible</span>)}</div>); }`
	out := TranspileJSX(input)
	if strings.Contains(out, "<span>") {
		t.Errorf("should transpile JSX inside conditionals, got: %s", out)
	}
}

func TestTranspileHyphenatedAttrs(t *testing.T) {
	input := `function App() { return (<line stroke-width="2" />); }`
	out := TranspileJSX(input)
	if !strings.Contains(out, `"stroke-width"`) {
		t.Errorf("should quote hyphenated attrs, got: %s", out)
	}
}

func TestTranspileFullNextPage(t *testing.T) {
	input := `"use client";
import { useState } from "react";
import Link from "next/link";

export default function Page() {
  const [count, setCount] = useState(0);
  return (
    <div className="container">
      <h1 onClick={() => setCount(count + 1)}>Count: {count}</h1>
      <Link href="/about">About</Link>
    </div>
  );
}`
	out := TranspileJSX(input)

	// The full pipeline: NextCompat + JSX → h() calls
	if strings.Contains(out, "className") {
		t.Error("should convert className")
	}
	if strings.Contains(out, "onClick") {
		t.Error("should convert onClick")
	}
	if strings.Contains(out, "<div") || strings.Contains(out, "<h1") {
		t.Errorf("should transpile all JSX, got: %s", out)
	}
	if !strings.Contains(out, `h("div"`) {
		t.Error("should have h() calls")
	}
	if !strings.Contains(out, `h("a"`) {
		t.Errorf("should convert Link to a, got: %s", out)
	}
	if strings.Contains(out, `"use client"`) {
		t.Error("should strip use client")
	}
}
