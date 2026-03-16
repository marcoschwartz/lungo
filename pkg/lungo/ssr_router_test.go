package lungo

import (
	"os"
	"strings"
	"testing"
)

// TestSSRUseRouterPathname verifies that useRouter().pathname returns the correct
// path during SSR layout evaluation, not just "/".
func TestSSRUseRouterPathname(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(dir+"/app/login", 0755)
	os.MkdirAll(dir+"/app/dashboard", 0755)
	os.MkdirAll(dir+"/static", 0755)

	// Layout that conditionally renders based on useRouter().pathname
	os.WriteFile(dir+"/app/layout.jsx", []byte(`
const { h, useRouter } = window.Lungo;

export default function Layout({ children }) {
  const router = useRouter();
  const isLogin = router.pathname === "/login";

  if (isLogin) {
    return (<div class="login-layout">{children}</div>);
  }
  return (
    <div class="app-layout">
      <aside class="sidebar">Nav</aside>
      <main>{children}</main>
    </div>
  );
}
`), 0644)

	os.WriteFile(dir+"/app/login/page.jsx", []byte(`
const { h } = window.Lungo;
export default function LoginPage() {
  return (<div class="login-form">Sign In</div>);
}
`), 0644)

	os.WriteFile(dir+"/app/dashboard/page.jsx", []byte(`
const { h } = window.Lungo;
export default function DashboardPage() {
  return (<div class="dashboard">Welcome</div>);
}
`), 0644)

	app := New(Options{AppDir: dir + "/app", StaticDir: dir + "/static", Dev: true})

	// Test /login — should get login-layout, NOT app-layout with sidebar
	t.Run("login_page_uses_login_layout", func(t *testing.T) {
		route := app.router.Match("/login")
		if route == nil {
			t.Fatal("no route for /login")
		}
		pageHTML, _, err := app.evaluatePageSSR(route.PagePath, nil, nil)
		if err != nil {
			t.Fatalf("evaluatePageSSR error: %v", err)
		}
		wrapped := app.wrapInLayouts(pageHTML, route.Layouts, false, "/login")
		t.Logf("Login wrapped: %s", wrapped)

		if !strings.Contains(wrapped, "login-layout") {
			t.Error("login page should use login-layout")
		}
		if strings.Contains(wrapped, "sidebar") {
			t.Error("login page should NOT have sidebar")
		}
		if !strings.Contains(wrapped, "Sign In") {
			t.Error("login page should contain Sign In")
		}
	})

	// Test /dashboard — should get app-layout with sidebar
	t.Run("dashboard_page_uses_app_layout", func(t *testing.T) {
		route := app.router.Match("/dashboard")
		if route == nil {
			t.Fatal("no route for /dashboard")
		}
		pageHTML, _, err := app.evaluatePageSSR(route.PagePath, nil, nil)
		if err != nil {
			t.Fatalf("evaluatePageSSR error: %v", err)
		}
		wrapped := app.wrapInLayouts(pageHTML, route.Layouts, false, "/dashboard")
		t.Logf("Dashboard wrapped: %s", wrapped)

		if !strings.Contains(wrapped, "app-layout") {
			t.Error("dashboard should use app-layout")
		}
		if !strings.Contains(wrapped, "sidebar") {
			t.Error("dashboard should have sidebar")
		}
		if !strings.Contains(wrapped, "Welcome") {
			t.Error("dashboard should contain Welcome")
		}
	})

	// Test default (no path) — should fall back to "/" behavior
	t.Run("default_path_is_root", func(t *testing.T) {
		route := app.router.Match("/dashboard")
		if route == nil {
			t.Fatal("no route for /dashboard")
		}
		pageHTML, _, _ := app.evaluatePageSSR(route.PagePath, nil, nil)
		// Without passing urlPath, should default to "/"
		wrapped := app.wrapInLayouts(pageHTML, route.Layouts, false)
		if !strings.Contains(wrapped, "app-layout") {
			t.Error("default should use app-layout (not login)")
		}
	})
}
