package lungo

import (
	"testing"

	"github.com/marcoschwartz/espresso"
)

// TestSSRScopeHasNumberBuiltin guards against a regression where the SSR
// scope was built without registering espresso's built-in globals. The
// bytecode VM uses ordinary scope lookup for call sites, so a missing
// Number, String, Boolean, parseInt, JSON, Math, Object, or Array was
// silently returning undefined — which broke component logic like
// `Number(level) || 2` and caused all <h${level}> tags to render as <h2>.
//
// If this test fails, check buildSSRScope and make sure it calls
// espresso.RegisterBuiltinGlobals before adding user-supplied values.
func TestSSRScopeHasNumberBuiltin(t *testing.T) {
	scope := buildSSRScope(nil)

	for _, name := range []string{"Number", "String", "Boolean", "parseInt", "parseFloat", "JSON", "Math", "Object", "Array"} {
		v, ok := scope[name]
		if !ok || v == nil || v.Type() != espresso.TypeFunc && v.Type() != espresso.TypeObject {
			t.Errorf("builtin %q missing or wrong type in SSR scope (ok=%v value=%v)", name, ok, v)
		}
	}

	// Callable check: Number("3") should return the number 3, not undefined.
	num := scope["Number"]
	if num == nil || num.Type() != espresso.TypeFunc {
		t.Fatal("Number not a function in SSR scope")
	}
	result := espresso.CallFunc(scope, num, map[string]*espresso.Value{
		"arg0": espresso.NewStr("3"),
	})
	if result == nil || result.Type() != espresso.TypeNumber {
		t.Fatalf("Number(\"3\") did not return a number: %v", result)
	}
}
