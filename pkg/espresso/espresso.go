package espresso

// VM is a JavaScript virtual machine instance.
// It evaluates JavaScript expressions and statements using a Go-native
// interpreter — no V8, no CGO, no external dependencies.
type VM struct {
	scope map[string]*Value
}

// New creates a new VM with an empty scope.
func New() *VM {
	return &VM{scope: make(map[string]*Value)}
}

// Set injects a Go value into the JS scope.
func (vm *VM) Set(name string, value interface{}) {
	vm.scope[name] = ToValue(value)
}

// SetValue injects a Value directly into the JS scope.
func (vm *VM) SetValue(name string, value *Value) {
	vm.scope[name] = value
}

// Get reads a value from scope.
func (vm *VM) Get(name string) *Value {
	if v, ok := vm.scope[name]; ok {
		return v
	}
	return Undefined
}

// Eval evaluates a JS expression and returns the result.
// Uses token caching — repeated calls with the same code skip tokenization.
func (vm *VM) Eval(code string) (*Value, error) {
	tokens := tokenizeCached(code)
	ev := &evaluator{tokens: tokens, pos: 0, scope: vm.scope}
	return ev.expr(), nil
}

// Run evaluates multiple JS statements (const, let, if, for, return, etc.).
// Returns the value of the last return statement, or Undefined.
func (vm *VM) Run(code string) (*Value, error) {
	tokens := tokenizeCached(code)
	ev := &evaluator{tokens: tokens, pos: 0, scope: vm.copyScope()}
	result := ev.evalStatements()
	for k, v := range ev.scope {
		vm.scope[k] = v
	}
	if result == nil || result == breakSentinel || result == continueSentinel {
		return Undefined, nil
	}
	return result, nil
}

// Call calls a function defined in scope with the given arguments.
func (vm *VM) Call(fn string, args ...interface{}) (*Value, error) {
	fnVal, ok := vm.scope[fn]
	if !ok || fnVal.typ != TypeFunc {
		return Undefined, nil
	}
	props := make(map[string]*Value)
	if len(fnVal.fnParams) == 1 && len(args) > 0 {
		props[fnVal.fnParams[0]] = ToValue(args[0])
	}
	ev := &evaluator{scope: vm.copyScope()}
	return ev.callFunc(fnVal, props), nil
}

// Compile pre-compiles JS expression code for fast repeated execution.
// The returned Compiled object can be executed many times without re-tokenizing.
//
//	compiled := vm.Compile("x * 2 + 1")
//	result := compiled.Exec(vm)
func (vm *VM) Compile(code string) *Compiled {
	return &Compiled{tokens: tokenizeCached(code), isExpr: true}
}

// CompileStatements pre-compiles JS statements for fast repeated execution.
func (vm *VM) CompileStatements(code string) *Compiled {
	return &Compiled{tokens: tokenizeCached(code), isExpr: false}
}

// Compiled is pre-compiled JS code that can be executed repeatedly
// without re-tokenization.
type Compiled struct {
	tokens []tok
	isExpr bool
}

// Exec executes the compiled code using the VM's scope.
func (c *Compiled) Exec(vm *VM) *Value {
	toks := make([]tok, len(c.tokens))
	copy(toks, c.tokens)
	if c.isExpr {
		ev := &evaluator{tokens: toks, pos: 0, scope: vm.scope}
		return ev.expr()
	}
	ev := &evaluator{tokens: toks, pos: 0, scope: vm.copyScope()}
	result := ev.evalStatements()
	for k, v := range ev.scope {
		vm.scope[k] = v
	}
	if result == nil || result == breakSentinel || result == continueSentinel {
		return Undefined
	}
	return result
}

// Scope returns a copy of all variables in the VM scope.
func (vm *VM) Scope() map[string]*Value {
	result := make(map[string]*Value, len(vm.scope))
	for k, v := range vm.scope {
		result[k] = v
	}
	return result
}

func (vm *VM) copyScope() map[string]*Value {
	scope := make(map[string]*Value, len(vm.scope))
	for k, v := range vm.scope {
		scope[k] = v
	}
	return scope
}
