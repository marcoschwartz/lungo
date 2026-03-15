package lungo

import (
	"fmt"
	"net/http"
	"strings"
)

// CORSOptions configures CORS middleware.
type CORSOptions struct {
	AllowOrigins []string
	AllowMethods []string
	AllowHeaders []string
	MaxAge       int // seconds
}

// CORS returns a middleware that handles Cross-Origin Resource Sharing.
func CORS(opts CORSOptions) Middleware {
	if len(opts.AllowMethods) == 0 {
		opts.AllowMethods = []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"}
	}
	if len(opts.AllowHeaders) == 0 {
		opts.AllowHeaders = []string{"Content-Type", "Authorization"}
	}
	if opts.MaxAge == 0 {
		opts.MaxAge = 86400
	}

	origin := "*"
	if len(opts.AllowOrigins) > 0 {
		origin = strings.Join(opts.AllowOrigins, ", ")
	}
	methods := strings.Join(opts.AllowMethods, ", ")
	headers := strings.Join(opts.AllowHeaders, ", ")

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Methods", methods)
			w.Header().Set("Access-Control-Allow-Headers", headers)

			if r.Method == http.MethodOptions {
				w.Header().Set("Access-Control-Max-Age", fmt.Sprintf("%d", opts.MaxAge))
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// RedirectRule defines a URL redirect.
type RedirectRule struct {
	From string
	To   string
	Code int // 301 or 302, defaults to 301
}

// Redirects returns a middleware that handles URL redirects.
func Redirects(rules []RedirectRule) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			for _, rule := range rules {
				if r.URL.Path == rule.From {
					code := rule.Code
					if code == 0 {
						code = http.StatusMovedPermanently
					}
					http.Redirect(w, r, rule.To, code)
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// BasicAuth returns a middleware that requires HTTP Basic Authentication.
func BasicAuth(username, password, realm string) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			u, p, ok := r.BasicAuth()
			if !ok || u != username || p != password {
				w.Header().Set("WWW-Authenticate", `Basic realm="`+realm+`"`)
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// PathGuard returns a middleware that runs a check function for paths matching
// the given prefix. If the check returns false, a 403 is returned.
// Useful for protecting routes (e.g., /admin/*).
func PathGuard(prefix string, check func(r *http.Request) bool) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasPrefix(r.URL.Path, prefix) {
				if !check(r) {
					http.Error(w, "Forbidden", http.StatusForbidden)
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}
