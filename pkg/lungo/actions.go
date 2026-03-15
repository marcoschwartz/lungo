package lungo

import (
	"encoding/json"
	"net/http"
	"strings"
)

// ActionResult is returned from a server action handler.
type ActionResult struct {
	// Redirect URL after action completes (optional).
	Redirect string

	// Data to pass back to the page (available as window.__LUNGO_ACTION_DATA__).
	Data interface{}

	// Error message to display.
	Error string
}

// Action registers a server action handler.
// Actions are POST endpoints at /action/{name} that handle form submissions.
// The handler receives the parsed form data and returns an ActionResult.
//
// Usage:
//
//	app.Action("contact", func(w http.ResponseWriter, r *http.Request) reactgo.ActionResult {
//	    email := r.FormValue("email")
//	    // process...
//	    return reactgo.ActionResult{Redirect: "/thank-you"}
//	})
//
// In page.js:
//
//	h`<form method="POST" action="/action/contact">
//	    <input name="email" type="email" />
//	    <button type="submit">Send</button>
//	</form>`
func (a *App) Action(name string, handler func(http.ResponseWriter, *http.Request) ActionResult) {
	a.apiRoutes["/action/"+name] = func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Parse form data
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Bad request", http.StatusBadRequest)
			return
		}

		result := handler(w, r)

		// If the request wants JSON (from fetch), return JSON
		if strings.Contains(r.Header.Get("Accept"), "application/json") {
			w.Header().Set("Content-Type", "application/json")
			if result.Error != "" {
				w.WriteHeader(http.StatusBadRequest)
				json.NewEncoder(w).Encode(map[string]string{"error": result.Error})
			} else {
				json.NewEncoder(w).Encode(map[string]interface{}{
					"redirect": result.Redirect,
					"data":     result.Data,
				})
			}
			return
		}

		// Regular form submission — redirect
		if result.Error != "" {
			// Redirect back with error
			referer := r.Header.Get("Referer")
			if referer == "" {
				referer = "/"
			}
			http.Redirect(w, r, referer+"?error="+result.Error, http.StatusSeeOther)
			return
		}

		if result.Redirect != "" {
			http.Redirect(w, r, result.Redirect, http.StatusSeeOther)
		} else {
			// Redirect back to the referring page
			referer := r.Header.Get("Referer")
			if referer == "" {
				referer = "/"
			}
			http.Redirect(w, r, referer, http.StatusSeeOther)
		}
	}
}
