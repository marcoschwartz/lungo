package lungo

import (
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"regexp"
	"strings"
)

// PageMetadata holds SEO metadata extracted from page.js files OR from
// per-request loader data. All fields are optional — empty means "don't emit
// that tag" (safer than guessing content).
type PageMetadata struct {
	Title       string           `json:"title,omitempty"`
	Description string           `json:"description,omitempty"`

	// Favicon URL — absolute URL or site-relative path. When set, emits
	// <link rel="icon"> + <link rel="apple-touch-icon">. SVG URLs get a
	// type="image/svg+xml" hint so browsers pick them over .ico siblings.
	// Empty = no icon link emitted (the browser will 404 on /favicon.ico
	// unless the host serves one itself).
	Favicon     string           `json:"favicon,omitempty"`

	// Open Graph + Twitter card. Conservative defaults:
	//   - og.image absent → no <meta property="og:image"> emitted
	//   - twitter.card absent → "summary_large_image" if og.image set, else omitted
	//   - og.type absent → "website"
	//   - canonical emitted only when explicit (no guessing)
	OG          *OpenGraphMeta   `json:"og,omitempty"`
	Twitter     *TwitterMeta     `json:"twitter,omitempty"`
	Canonical   string           `json:"canonical,omitempty"`
}

// OpenGraphMeta drives <meta property="og:*"> tags.
type OpenGraphMeta struct {
	Title       string `json:"title,omitempty"`       // falls back to Title
	Description string `json:"description,omitempty"` // falls back to Description
	Image       string `json:"image,omitempty"`
	Type        string `json:"type,omitempty"`        // default "website"
	Locale      string `json:"locale,omitempty"`
	SiteName    string `json:"site_name,omitempty"`
	URL         string `json:"url,omitempty"`         // defaults to current request URL
}

// TwitterMeta drives <meta name="twitter:*"> tags.
type TwitterMeta struct {
	Card        string `json:"card,omitempty"`        // default based on og.image
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	Image       string `json:"image,omitempty"`
	Site        string `json:"site,omitempty"`        // @handle
	Creator     string `json:"creator,omitempty"`
}

// extractMetadata reads a page.js file and extracts the metadata export.
// Looks for: export const metadata = { title: "...", description: "..." }
func (a *App) extractMetadata(pagePath string) *PageMetadata {
	data, err := a.readAppFile(pagePath)
	if err != nil {
		return nil
	}
	content := string(data)

	idx := strings.Index(content, "export const metadata")
	if idx < 0 {
		idx = strings.Index(content, "export let metadata")
	}
	if idx < 0 {
		return nil
	}

	// Find the object literal
	rest := content[idx:]
	braceStart := strings.Index(rest, "{")
	if braceStart < 0 {
		return nil
	}
	rest = rest[braceStart:]

	// Find matching closing brace
	depth := 0
	end := -1
	for i, ch := range rest {
		if ch == '{' {
			depth++
		} else if ch == '}' {
			depth--
			if depth == 0 {
				end = i + 1
				break
			}
		}
	}
	if end < 0 {
		return nil
	}

	objStr := rest[:end]
	// Convert JS object to JSON (handle unquoted keys)
	jsonStr := jsObjToJSON(objStr)

	var meta PageMetadata
	if err := json.Unmarshal([]byte(jsonStr), &meta); err != nil {
		return nil
	}
	return &meta
}

// jsObjToJSON converts a simple JS object literal to JSON.
// Handles: { title: "Hello", description: "World" }
// Does NOT handle computed keys, nested objects, etc.
func jsObjToJSON(s string) string {
	var result strings.Builder
	i := 0
	inString := false
	stringChar := byte(0)

	for i < len(s) {
		ch := s[i]

		if inString {
			result.WriteByte(ch)
			if ch == stringChar && (i == 0 || s[i-1] != '\\') {
				inString = false
			}
			i++
			continue
		}

		if ch == '"' || ch == '\'' {
			if ch == '\'' {
				result.WriteByte('"') // convert single quotes to double
			} else {
				result.WriteByte(ch)
			}
			inString = true
			stringChar = ch
			i++
			continue
		}

		// Check for unquoted key: word followed by :
		if ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch == '_' {
			keyStart := i
			for i < len(s) && (s[i] >= 'a' && s[i] <= 'z' || s[i] >= 'A' && s[i] <= 'Z' || s[i] >= '0' && s[i] <= '9' || s[i] == '_') {
				i++
			}
			key := s[keyStart:i]
			// Skip whitespace
			for i < len(s) && (s[i] == ' ' || s[i] == '\t' || s[i] == '\n' || s[i] == '\r') {
				i++
			}
			if i < len(s) && s[i] == ':' {
				// It's a key — quote it
				result.WriteByte('"')
				result.WriteString(key)
				result.WriteByte('"')
			} else {
				result.WriteString(key)
			}
			continue
		}

		result.WriteByte(ch)
		i++
	}

	// Remove trailing commas before } or ] (invalid in JSON but valid in JS)
	out := result.String()
	trailingComma := regexp.MustCompile(`,\s*([}\]])`)
	out = trailingComma.ReplaceAllString(out, "$1")
	return out
}

// renderMetadataHead generates <head> tags from metadata.
func renderMetadataHead(meta *PageMetadata) string {
	if meta == nil {
		return ""
	}
	var sb strings.Builder
	if meta.Title != "" {
		sb.WriteString("  <title>")
		sb.WriteString(html.EscapeString(meta.Title))
		sb.WriteString("</title>\n")
	}
	if meta.Description != "" {
		sb.WriteString("  <meta name=\"description\" content=\"")
		sb.WriteString(html.EscapeString(meta.Description))
		sb.WriteString("\">\n")
	}
	if meta.Favicon != "" {
		href := html.EscapeString(meta.Favicon)
		// Hint the MIME type for SVG so browsers prefer it over a parallel .ico.
		if strings.HasSuffix(strings.ToLower(meta.Favicon), ".svg") {
			sb.WriteString("  <link rel=\"icon\" type=\"image/svg+xml\" href=\"")
		} else {
			sb.WriteString("  <link rel=\"icon\" href=\"")
		}
		sb.WriteString(href)
		sb.WriteString("\">\n")
		sb.WriteString("  <link rel=\"apple-touch-icon\" href=\"")
		sb.WriteString(href)
		sb.WriteString("\">\n")
	}
	return sb.String()
}

// renderSocialMetaHead renders Open Graph + Twitter Card + canonical tags.
// Conservative rendering — empty fields emit nothing. Falls back only where
// safe: og:title → meta.Title, og:description → meta.Description,
// og:type → "website", twitter:card → "summary_large_image" when an image
// is present (else omitted).
//
// Request is used only to fill og:url when it's not explicitly set AND
// meta.OG itself is non-nil (avoids emitting a bare og:url with no context).
func renderSocialMetaHead(meta *PageMetadata, r *http.Request) string {
	if meta == nil {
		return ""
	}
	var sb strings.Builder
	write := func(tag, name, content string) {
		if content == "" {
			return
		}
		sb.WriteString("  <meta ")
		sb.WriteString(tag)
		sb.WriteString("=\"")
		sb.WriteString(name)
		sb.WriteString("\" content=\"")
		sb.WriteString(html.EscapeString(content))
		sb.WriteString("\">\n")
	}

	// Open Graph. Emit the block when any OG-relevant field exists —
	// og:title + og:description fall back to the page's regular title/desc
	// so a plain Hero page still gets a nice preview card.
	og := meta.OG
	if og != nil || meta.Title != "" || meta.Description != "" {
		ogTitle := meta.Title
		if og != nil && og.Title != "" {
			ogTitle = og.Title
		}
		ogDesc := meta.Description
		if og != nil && og.Description != "" {
			ogDesc = og.Description
		}
		ogType := "website"
		if og != nil && og.Type != "" {
			ogType = og.Type
		}
		ogURL := ""
		ogImage, ogLocale, ogSiteName := "", "", ""
		if og != nil {
			ogURL = og.URL
			ogImage = og.Image
			ogLocale = og.Locale
			ogSiteName = og.SiteName
		}
		if ogURL == "" && r != nil {
			ogURL = requestURL(r)
		}
		write("property", "og:title", ogTitle)
		write("property", "og:description", ogDesc)
		write("property", "og:type", ogType)
		write("property", "og:url", ogURL)
		write("property", "og:site_name", ogSiteName)
		write("property", "og:locale", ogLocale)
		if ogImage != "" {
			write("property", "og:image", ogImage)
			// Standard social-card dimensions; callers can override by
			// setting explicit meta tags via HeadExtra if needed.
			sb.WriteString("  <meta property=\"og:image:width\" content=\"1200\">\n")
			sb.WriteString("  <meta property=\"og:image:height\" content=\"630\">\n")
		}
	}

	// Twitter Card. Defaults to the OG image if nothing more specific.
	tw := meta.Twitter
	hasOGImage := meta.OG != nil && meta.OG.Image != ""
	if tw != nil || hasOGImage {
		card := "summary"
		if tw != nil && tw.Card != "" {
			card = tw.Card
		} else if hasOGImage {
			card = "summary_large_image"
		}
		twTitle, twDesc, twImage, twSite, twCreator := "", "", "", "", ""
		if tw != nil {
			twTitle, twDesc, twImage = tw.Title, tw.Description, tw.Image
			twSite, twCreator = tw.Site, tw.Creator
		}
		if twImage == "" && hasOGImage {
			twImage = meta.OG.Image
		}
		write("name", "twitter:card", card)
		write("name", "twitter:title", firstNonEmpty(twTitle, meta.Title))
		write("name", "twitter:description", firstNonEmpty(twDesc, meta.Description))
		write("name", "twitter:image", twImage)
		write("name", "twitter:site", twSite)
		write("name", "twitter:creator", twCreator)
	}

	// Canonical — only when explicitly set. Emitting a guess can hurt SEO.
	if meta.Canonical != "" {
		sb.WriteString(fmt.Sprintf("  <link rel=\"canonical\" href=\"%s\">\n",
			html.EscapeString(meta.Canonical)))
	}

	return sb.String()
}

func requestURL(r *http.Request) string {
	proto := r.Header.Get("X-Forwarded-Proto")
	if proto == "" {
		if r.TLS != nil {
			proto = "https"
		} else {
			proto = "http"
		}
	}
	host := r.Host
	if xfh := r.Header.Get("X-Forwarded-Host"); xfh != "" {
		host = xfh
	}
	return proto + "://" + host + r.URL.Path
}

func firstNonEmpty(vs ...string) string {
	for _, v := range vs {
		if v != "" {
			return v
		}
	}
	return ""
}
