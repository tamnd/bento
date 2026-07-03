package goimport

import (
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"slices"
	"testing"
)

// checkSource type-checks a single Go source file and returns its package, so the
// mapping tests run against the real go/types view of a signature rather than a
// hand-built type. Imports resolve through the compiler's export data, so standard
// library types like io.Reader and context.Context are the genuine ones.
func checkSource(t *testing.T, src string) *types.Package {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "src.go", src, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	conf := types.Config{Importer: importer.Default()}
	pkg, err := conf.Check("p", fset, []*ast.File{f}, nil)
	if err != nil {
		t.Fatalf("typecheck: %v", err)
	}
	return pkg
}

// lookupType returns the declared type named in the package, so a test can name a
// symbol in source and assert on how the mapper projects its type.
func lookupType(t *testing.T, pkg *types.Package, name string) types.Type {
	t.Helper()
	obj := pkg.Scope().Lookup(name)
	if obj == nil {
		t.Fatalf("no symbol %q in package", name)
	}
	return obj.Type()
}

// funcResults maps the result of a named top-level function through the shared
// throw-mode result rule, which is what the declaration emitter will call.
func funcResults(m *Mapper, pkg *types.Package, name string) string {
	sig := pkg.Scope().Lookup(name).Type().(*types.Signature)
	return m.mapResults(sig.Results())
}

func TestMapPrimitives(t *testing.T) {
	src := `package p
var B bool
var S string
var I int
var I64 int64
var U8 uint8
var F32 float32
var F64 float64
`
	pkg := checkSource(t, src)
	m := NewMapper(pkg)
	cases := map[string]string{
		"B":   "boolean",
		"S":   "string",
		"I":   "number",
		"I64": "number",
		"U8":  "number",
		"F32": "number",
		"F64": "number",
	}
	for name, want := range cases {
		if got := m.Map(lookupType(t, pkg, name)); got != want {
			t.Errorf("%s projected to %q, want %q", name, got, want)
		}
	}
}

func TestMapSlicesAndBytes(t *testing.T) {
	src := `package p
var Names []string
var Data []byte
var Points [][]int32
`
	pkg := checkSource(t, src)
	m := NewMapper(pkg)
	if got := m.Map(lookupType(t, pkg, "Names")); got != "string[]" {
		t.Errorf("[]string projected to %q, want string[]", got)
	}
	if got := m.Map(lookupType(t, pkg, "Data")); got != "Uint8Array" {
		t.Errorf("[]byte projected to %q, want Uint8Array", got)
	}
	if got := m.Map(lookupType(t, pkg, "Points")); got != "number[][]" {
		t.Errorf("[][]int32 projected to %q, want number[][]", got)
	}
}

func TestMapMap(t *testing.T) {
	src := `package p
var Headers map[string]string
var Counts map[string]int
`
	pkg := checkSource(t, src)
	m := NewMapper(pkg)
	if got := m.Map(lookupType(t, pkg, "Headers")); got != "Map<string, string>" {
		t.Errorf("map[string]string projected to %q", got)
	}
	if got := m.Map(lookupType(t, pkg, "Counts")); got != "Map<string, number>" {
		t.Errorf("map[string]int projected to %q", got)
	}
}

func TestMapPointerIsTransparent(t *testing.T) {
	src := `package p
type Decoder struct{ x int }
var D *Decoder
`
	pkg := checkSource(t, src)
	m := NewMapper(pkg)
	if got := m.Map(lookupType(t, pkg, "D")); got != "Decoder" {
		t.Errorf("*Decoder projected to %q, want Decoder (pointer is transparent)", got)
	}
}

func TestMapChannel(t *testing.T) {
	src := `package p
var Events chan int
`
	pkg := checkSource(t, src)
	m := NewMapper(pkg)
	if got := m.Map(lookupType(t, pkg, "Events")); got != "GoChannel<number>" {
		t.Errorf("chan int projected to %q, want GoChannel<number>", got)
	}
	if !usesHelper(m, HelperChannel) {
		t.Errorf("channel projection did not record the GoChannel helper")
	}
}

func TestMapWellKnownInterfaces(t *testing.T) {
	src := `package p
import "io"
import "context"
var R io.Reader
var W io.Writer
var RC io.ReadCloser
var Ctx context.Context
`
	pkg := checkSource(t, src)
	m := NewMapper(pkg)
	cases := map[string]string{
		"R":   "GoReader",
		"W":   "GoWriter",
		"RC":  "GoReadCloser",
		"Ctx": "GoContext",
	}
	for name, want := range cases {
		if got := m.Map(lookupType(t, pkg, name)); got != want {
			t.Errorf("%s projected to %q, want %q", name, got, want)
		}
	}
}

func TestMapForeignNamedIsOpaque(t *testing.T) {
	src := `package p
import "time"
var D time.Duration
`
	pkg := checkSource(t, src)
	m := NewMapper(pkg)
	// time.Duration is a foreign named type with no well-known projection, so it
	// crosses as an opaque token tagged with its Go name.
	if got := m.Map(lookupType(t, pkg, "D")); got != `GoOpaque<"time.Duration">` {
		t.Errorf("time.Duration projected to %q, want the opaque handle", got)
	}
	if !usesHelper(m, HelperOpaque) {
		t.Errorf("opaque projection did not record the GoOpaque helper")
	}
}

func TestMapLocalNamedByBareName(t *testing.T) {
	src := `package p
type Point struct{ X, Y float64 }
var Ps []Point
`
	pkg := checkSource(t, src)
	m := NewMapper(pkg)
	if got := m.Map(lookupType(t, pkg, "Ps")); got != "Point[]" {
		t.Errorf("[]Point projected to %q, want Point[]", got)
	}
}

func TestMapResultsThrowMode(t *testing.T) {
	src := `package p
func Parse(s string) (int, error) { return 0, nil }
func Close() error { return nil }
func Div(a, b int) (int, int) { return 0, 0 }
func Nothing() {}
`
	pkg := checkSource(t, src)
	m := NewMapper(pkg)
	// A single non-error result with a trailing error hoists to just the value.
	if got := funcResults(m, pkg, "Parse"); got != "number" {
		t.Errorf("(int, error) result projected to %q, want number", got)
	}
	// A lone error result becomes a throw, so the result type is void.
	if got := funcResults(m, pkg, "Close"); got != "void" {
		t.Errorf("(error) result projected to %q, want void", got)
	}
	// Two non-error results with nothing to hoist stay a tuple.
	if got := funcResults(m, pkg, "Div"); got != "[number, number]" {
		t.Errorf("(int, int) result projected to %q, want [number, number]", got)
	}
	if got := funcResults(m, pkg, "Nothing"); got != "void" {
		t.Errorf("no result projected to %q, want void", got)
	}
}

func TestMapFuncTypeWithCallbackAndVariadic(t *testing.T) {
	src := `package p
type FileInfo struct{ name string }
var Fn func(path string, info FileInfo) error
var VarFn func(parts ...string) int
`
	pkg := checkSource(t, src)
	m := NewMapper(pkg)
	if got := m.Map(lookupType(t, pkg, "Fn")); got != "(path: string, info: FileInfo) => void" {
		t.Errorf("callback projected to %q", got)
	}
	if got := m.Map(lookupType(t, pkg, "VarFn")); got != "(...parts: string[]) => number" {
		t.Errorf("variadic func projected to %q", got)
	}
}

// TestParamListRenamesReservedNames proves a Go parameter whose name is a
// TypeScript reserved word (strings.Replace has a parameter literally named `new`)
// is renamed to a positional label, so the emitted declaration parses. A parameter
// name is only a label the type rides on, never referenced by a caller, so the
// rename is invisible to the projected surface.
func TestParamListRenamesReservedNames(t *testing.T) {
	src := `package p
var Fn func(old string, new string, function int) string
`
	pkg := checkSource(t, src)
	m := NewMapper(pkg)
	got := m.Map(lookupType(t, pkg, "Fn"))
	want := "(old: string, a1: string, a2: number) => string"
	if got != want {
		t.Errorf("reserved-word params projected to %q, want %q", got, want)
	}
}

func TestMapUnsafeIsUnsupported(t *testing.T) {
	src := `package p
import "unsafe"
var P unsafe.Pointer
var C complex128
`
	pkg := checkSource(t, src)
	m := NewMapper(pkg)
	if got := m.Map(lookupType(t, pkg, "P")); got != "GoUnsupported" {
		t.Errorf("unsafe.Pointer projected to %q, want GoUnsupported", got)
	}
	if got := m.Map(lookupType(t, pkg, "C")); got != "GoUnsupported" {
		t.Errorf("complex128 projected to %q, want GoUnsupported", got)
	}
	if !usesHelper(m, HelperUnsupported) {
		t.Errorf("unsupported projection did not record the GoUnsupported helper")
	}
}

func TestUsedHelpersAreOrderedAndDeduped(t *testing.T) {
	src := `package p
import "io"
var R io.Reader
var W io.Writer
var C chan int
var C2 chan string
`
	pkg := checkSource(t, src)
	m := NewMapper(pkg)
	for _, name := range []string{"R", "W", "C", "C2"} {
		m.Map(lookupType(t, pkg, name))
	}
	used := m.Used()
	want := []Helper{HelperChannel, HelperReader, HelperWriter}
	if len(used) != len(want) {
		t.Fatalf("used helpers = %v, want %v", used, want)
	}
	for i := range want {
		if used[i] != want[i] {
			t.Fatalf("used helpers = %v, want %v (order matters)", used, want)
		}
	}
}

func usesHelper(m *Mapper, h Helper) bool {
	return slices.Contains(m.Used(), h)
}
