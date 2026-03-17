// Package espresso is a lightweight JavaScript evaluator written in pure Go.
// It supports most common JS patterns: variables, operators, functions, arrows,
// closures, array/string/object methods, template literals, loops, try/catch,
// and more — without any external dependencies.
package espresso

import (
	"encoding/json"
	"strconv"
	"strings"
)

// Type represents a JavaScript value type.
type Type int

const (
	TypeUndefined Type = iota
	TypeNull
	TypeBool
	TypeNumber
	TypeString
	TypeArray
	TypeObject
	TypeFunc
)

// Value represents a JavaScript value.
type Value struct {
	typ      Type
	bool     bool
	num      float64
	str      string
	array    []*Value
	object   map[string]*Value
	fnParams []string
	fnBody   string
}

// Undefined is the JS undefined value.
var Undefined = &Value{typ: TypeUndefined}

// Null is the JS null value.
var Null = &Value{typ: TypeNull}

// True is the JS true value.
var True = &Value{typ: TypeBool, bool: true}

// False is the JS false value.
var False = &Value{typ: TypeBool, bool: false}

// String returns the string representation of the value.
func (v *Value) String() string {
	if v == nil {
		return "undefined"
	}
	return v.toStr()
}

// Number returns the numeric value.
func (v *Value) Number() float64 {
	if v == nil {
		return 0
	}
	return v.toNum()
}

// Bool returns the boolean value.
func (v *Value) Bool() bool {
	if v == nil {
		return false
	}
	return v.bool
}

// Type returns the value's type.
func (v *Value) Type() Type {
	if v == nil {
		return TypeUndefined
	}
	return v.typ
}

// IsNull returns true if the value is null.
func (v *Value) IsNull() bool { return v != nil && v.typ == TypeNull }

// IsUndefined returns true if the value is undefined.
func (v *Value) IsUndefined() bool { return v == nil || v.typ == TypeUndefined }

// IsArray returns true if the value is an array.
func (v *Value) IsArray() bool { return v != nil && v.typ == TypeArray }

// IsObject returns true if the value is an object.
func (v *Value) IsObject() bool { return v != nil && v.typ == TypeObject }

// Truthy returns the JS truthiness of the value.
func (v *Value) Truthy() bool {
	if v == nil {
		return false
	}
	return v.truthy()
}

// Get returns a property of an object or array element.
func (v *Value) Get(key string) *Value {
	if v == nil {
		return Undefined
	}
	return v.getProp(key)
}

// Array returns the array elements as a slice.
func (v *Value) Array() []*Value {
	if v == nil || v.typ != TypeArray {
		return nil
	}
	return v.array
}

// Object returns the object properties as a map.
func (v *Value) Object() map[string]*Value {
	if v == nil || v.typ != TypeObject {
		return nil
	}
	return v.object
}

// Len returns the length of an array or string.
func (v *Value) Len() int {
	if v == nil {
		return 0
	}
	if v.typ == TypeArray {
		return len(v.array)
	}
	if v.typ == TypeString {
		return len(v.str)
	}
	return 0
}

// Interface converts the Value to a native Go type.
func (v *Value) Interface() interface{} {
	return valueToInterface(v)
}

// ── Internal helpers ────────────────────────────────────

func (v *Value) truthy() bool {
	switch v.typ {
	case TypeUndefined, TypeNull:
		return false
	case TypeBool:
		return v.bool
	case TypeNumber:
		return v.num != 0
	case TypeString:
		return v.str != ""
	case TypeArray, TypeObject, TypeFunc:
		return true
	}
	return false
}

func (v *Value) toStr() string {
	switch v.typ {
	case TypeUndefined:
		return "undefined"
	case TypeNull:
		return "null"
	case TypeBool:
		if v.bool {
			return "true"
		}
		return "false"
	case TypeNumber:
		if v.num == float64(int64(v.num)) {
			return strconv.FormatInt(int64(v.num), 10)
		}
		return strconv.FormatFloat(v.num, 'f', -1, 64)
	case TypeString:
		return v.str
	case TypeArray:
		var parts []string
		for _, item := range v.array {
			parts = append(parts, item.toStr())
		}
		return strings.Join(parts, ",")
	case TypeObject:
		b, _ := json.Marshal(valueToInterface(v))
		return string(b)
	}
	return ""
}

func (v *Value) toNum() float64 {
	switch v.typ {
	case TypeUndefined:
		return 0
	case TypeNull:
		return 0
	case TypeBool:
		if v.bool {
			return 1
		}
		return 0
	case TypeNumber:
		return v.num
	case TypeString:
		n, err := strconv.ParseFloat(strings.TrimSpace(v.str), 64)
		if err != nil {
			return 0
		}
		return n
	}
	return 0
}

func (v *Value) getProp(key string) *Value {
	if v.typ == TypeObject && v.object != nil {
		if val, ok := v.object[key]; ok {
			return val
		}
		return Undefined
	}
	if v.typ == TypeArray {
		if key == "length" {
			return newNum(float64(len(v.array)))
		}
		idx, err := strconv.Atoi(key)
		if err == nil && idx >= 0 && idx < len(v.array) {
			return v.array[idx]
		}
		return Undefined
	}
	if v.typ == TypeString {
		if key == "length" {
			return newNum(float64(len(v.str)))
		}
		idx, err := strconv.Atoi(key)
		if err == nil && idx >= 0 && idx < len(v.str) {
			return newStr(string(v.str[idx]))
		}
		return Undefined
	}
	return Undefined
}

// ── Constructors ────────────────────────────────────────

func newStr(s string) *Value     { return &Value{typ: TypeString, str: s} }
func newNum(n float64) *Value    { return &Value{typ: TypeNumber, num: n} }
func newBool(b bool) *Value      { if b { return True }; return False }
func newArr(a []*Value) *Value   { return &Value{typ: TypeArray, array: a} }
func newObj(o map[string]*Value) *Value { return &Value{typ: TypeObject, object: o} }

func strictEqual(a, b *Value) bool {
	if a.typ != b.typ {
		return false
	}
	switch a.typ {
	case TypeUndefined, TypeNull:
		return true
	case TypeBool:
		return a.bool == b.bool
	case TypeNumber:
		return a.num == b.num
	case TypeString:
		return a.str == b.str
	}
	return a == b // reference equality for objects/arrays
}

func valueToInterface(v *Value) interface{} {
	if v == nil {
		return nil
	}
	switch v.typ {
	case TypeUndefined, TypeNull:
		return nil
	case TypeBool:
		return v.bool
	case TypeNumber:
		return v.num
	case TypeString:
		return v.str
	case TypeArray:
		arr := make([]interface{}, len(v.array))
		for i, item := range v.array {
			arr[i] = valueToInterface(item)
		}
		return arr
	case TypeObject:
		obj := make(map[string]interface{}, len(v.object))
		for k, val := range v.object {
			obj[k] = valueToInterface(val)
		}
		return obj
	}
	return nil
}

// ToValue converts a Go value to a JS Value.
func ToValue(v interface{}) *Value {
	if v == nil {
		return Null
	}
	switch val := v.(type) {
	case bool:
		return newBool(val)
	case int:
		return newNum(float64(val))
	case int64:
		return newNum(float64(val))
	case float64:
		return newNum(val)
	case string:
		return newStr(val)
	case []interface{}:
		arr := make([]*Value, len(val))
		for i, item := range val {
			arr[i] = ToValue(item)
		}
		return newArr(arr)
	case map[string]interface{}:
		obj := make(map[string]*Value, len(val))
		for k, item := range val {
			obj[k] = ToValue(item)
		}
		return newObj(obj)
	case json.RawMessage:
		var raw interface{}
		json.Unmarshal(val, &raw)
		return ToValue(raw)
	case *Value:
		return val
	}
	return Undefined
}
