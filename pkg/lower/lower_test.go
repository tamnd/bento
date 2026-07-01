package lower

import (
	"errors"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tamnd/bento/pkg/frontend"
	"github.com/tamnd/bento/pkg/frontend/adapter"
)

var update = flag.Bool("update", false, "rewrite testdata golden files")

// renderOf builds a renderer over a one-type program and renders that type, so a
// test can assert the Go expression for a primitive, array, or object shape. The
// type is attached to a throwaway node the renderer never inspects; RenderType
// reads the type directly.
func renderOf(t *testing.T, ty *adapter.FakeType) (*Renderer, string, error) {
	t.Helper()
	f := adapter.NewFake()
	node := f.Node(adapter.NodeVariableDeclaration, ty)
	a, handle := f.Program(node)
	prog := frontend.Wrap(a, handle)
	r := NewRenderer(prog)
	got, err := r.RenderType(prog.TypeAt(prog.SourceFiles()[0]))
	return r, got, err
}

// TestPrimitiveTypesRender pins the section 3 to 8 primitive mappings, the clean
// core of the mapping table.
func TestPrimitiveTypesRender(t *testing.T) {
	cases := []struct {
		name  string
		flags frontend.TypeFlags
		want  string
	}{
		{"number", adapter.TypeNumber, "float64"},
		{"bigint", adapter.TypeBigInt, "*big.Int"},
		{"string", adapter.TypeString, "bstr"},
		{"boolean", adapter.TypeBoolean, "bool"},
		{"symbol", adapter.TypeSymbol, "*value.Symbol"},
	}
	f := adapter.NewFake()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, got, err := renderOf(t, f.Prim(tc.flags))
			if err != nil {
				t.Fatalf("RenderType(%s) error: %v", tc.name, err)
			}
			if got != tc.want {
				t.Errorf("RenderType(%s) = %q, want %q", tc.name, got, tc.want)
			}
		})
	}
}

// TestArrayRendersToHeader pins section 11: an array lowers to the Array[T]
// header by default, the correctness form, since the bare-slice fast path is a
// partitioner-proven optimization that is a later slice.
func TestArrayRendersToHeader(t *testing.T) {
	f := adapter.NewFake()
	_, got, err := renderOf(t, f.Array(f.Prim(adapter.TypeNumber)))
	if err != nil {
		t.Fatalf("RenderType(number[]) error: %v", err)
	}
	if want := "*value.Array[float64]"; got != want {
		t.Errorf("RenderType(number[]) = %q, want %q", got, want)
	}
}

// TestNestedArrayRecurses proves the element type is lowered by the same rules,
// so string[][] nests the header.
func TestNestedArrayRecurses(t *testing.T) {
	f := adapter.NewFake()
	inner := f.Array(f.Prim(adapter.TypeString))
	_, got, err := renderOf(t, f.Array(inner))
	if err != nil {
		t.Fatalf("RenderType(string[][]) error: %v", err)
	}
	if want := "*value.Array[*value.Array[bstr]]"; got != want {
		t.Errorf("RenderType(string[][]) = %q, want %q", got, want)
	}
}

// TestObjectRendersToStructPointer pins section 12: a fixed-shape object lowers
// to a pointer to a generated struct, and the struct declaration is emitted with
// exported fields in declaration order.
func TestObjectRendersToStructPointer(t *testing.T) {
	f := adapter.NewFake()
	num := f.Prim(adapter.TypeNumber)
	point := f.Object(f.Prop("x", num), f.Prop("y", num))

	r, got, err := renderOf(t, point)
	if err != nil {
		t.Fatalf("RenderType(point) error: %v", err)
	}
	if want := "*ObjXY"; got != want {
		t.Errorf("RenderType(point) = %q, want %q", got, want)
	}
	decls := r.Decls()
	if len(decls) != 1 {
		t.Fatalf("got %d decls, want 1", len(decls))
	}
	checkGolden(t, "point_struct.golden", decls[0].Source)
}

// TestObjectFieldTypesLower proves nested object and primitive fields each lower
// by their own rule, so a struct field of object type is a pointer to that
// field's own generated struct.
func TestObjectFieldTypesLower(t *testing.T) {
	f := adapter.NewFake()
	num := f.Prim(adapter.TypeNumber)
	str := f.Prim(adapter.TypeString)
	point := f.Object(f.Prop("x", num), f.Prop("y", num))
	shape := f.Object(f.Prop("origin", point), f.Prop("label", str))

	r, got, err := renderOf(t, shape)
	if err != nil {
		t.Fatalf("RenderType(shape) error: %v", err)
	}
	if want := "*ObjLabelOrigin"; got != want {
		t.Errorf("RenderType(shape) = %q, want %q", got, want)
	}
	decls := r.Decls()
	if len(decls) != 2 {
		t.Fatalf("got %d decls, want 2 (the outer shape and the nested point)", len(decls))
	}
	// Concatenate both declarations for one golden, so field order and the
	// nested pointer field are both pinned.
	var all strings.Builder
	for _, d := range decls {
		all.WriteString(d.Source)
	}
	checkGolden(t, "nested_struct.golden", all.String())
}

// TestSameShapeInternsToOneStruct pins the interning rule of section 12: the
// same structural shape, surfaced as the same frontend type identity, lowers to
// one Go struct with one declaration, not two.
func TestSameShapeInternsToOneStruct(t *testing.T) {
	f := adapter.NewFake()
	num := f.Prim(adapter.TypeNumber)
	point := f.Object(f.Prop("x", num), f.Prop("y", num))

	// A shape that uses the same point type in two fields must emit the point
	// struct exactly once.
	pair := f.Object(f.Prop("a", point), f.Prop("b", point))
	r, _, err := renderOf(t, pair)
	if err != nil {
		t.Fatalf("RenderType(pair) error: %v", err)
	}
	names := map[string]int{}
	for _, d := range r.Decls() {
		names[d.Name]++
	}
	if names["ObjXY"] != 1 {
		t.Errorf("ObjXY emitted %d times, want exactly 1 (interned)", names["ObjXY"])
	}
}

// TestDistinctShapesShareBaseNameGetSuffix proves the collision rule of section
// 29: two different shapes that derive the same base name get deterministic,
// distinct Go names.
func TestDistinctShapesShareBaseNameGetSuffix(t *testing.T) {
	f := adapter.NewFake()
	// Both shapes have a single field named x, so both derive base name ObjX,
	// but their field types differ, so they are distinct structs.
	xNum := f.Object(f.Prop("x", f.Prim(adapter.TypeNumber)))
	xStr := f.Object(f.Prop("x", f.Prim(adapter.TypeString)))
	outer := f.Object(f.Prop("a", xNum), f.Prop("b", xStr))

	r, _, err := renderOf(t, outer)
	if err != nil {
		t.Fatalf("RenderType(outer) error: %v", err)
	}
	seen := map[string]bool{}
	for _, d := range r.Decls() {
		if seen[d.Name] {
			t.Fatalf("duplicate generated name %q", d.Name)
		}
		seen[d.Name] = true
	}
	if !seen["ObjX"] || !seen["ObjX_2"] {
		t.Errorf("want both ObjX and ObjX_2, got names %v", seen)
	}
}

// TestStringLiteralUnionRendersToEnum pins section 10: a closed union of string
// literals lowers to a small integer tag enum named from its members, and the
// generated type plus its const block is emitted once. The members are given out
// of sorted order to prove tag assignment is by value, not by checker order.
func TestStringLiteralUnionRendersToEnum(t *testing.T) {
	f := adapter.NewFake()
	shape := f.Union(f.StringLit("rect"), f.StringLit("circle"))

	r, got, err := renderOf(t, shape)
	if err != nil {
		t.Fatalf("RenderType(union) error: %v", err)
	}
	if want := "LitCircleRect"; got != want {
		t.Errorf("RenderType(union) = %q, want %q", got, want)
	}
	decls := r.Decls()
	if len(decls) != 1 {
		t.Fatalf("got %d decls, want 1", len(decls))
	}
	checkGolden(t, "string_enum.golden", decls[0].Source)
}

// TestSameStringUnionInternsToOneEnum pins that the same set of string literals,
// surfaced as one type identity, lowers to one enum, so a type used in two places
// does not emit its enum twice.
func TestSameStringUnionInternsToOneEnum(t *testing.T) {
	f := adapter.NewFake()
	dir := f.Union(f.StringLit("north"), f.StringLit("south"))
	pair := f.Object(f.Prop("from", dir), f.Prop("to", dir))

	r, _, err := renderOf(t, pair)
	if err != nil {
		t.Fatalf("RenderType(pair) error: %v", err)
	}
	names := map[string]int{}
	for _, d := range r.Decls() {
		names[d.Name]++
	}
	if names["LitNorthSouth"] != 1 {
		t.Errorf("LitNorthSouth emitted %d times, want exactly 1 (interned)", names["LitNorthSouth"])
	}
}

// TestNonIdentifierStringUnionHandsBack pins that a string-literal union whose
// member is not a Go identifier has no clean tag name yet, so it hands back
// rather than invent one.
func TestNonIdentifierStringUnionHandsBack(t *testing.T) {
	f := adapter.NewFake()
	u := f.Union(f.StringLit("north"), f.StringLit("due east"))
	_, _, err := renderOf(t, u)
	var nyl *NotYetLowerable
	if !errors.As(err, &nyl) {
		t.Fatalf("RenderType(union with a spaced member) err = %v, want a *NotYetLowerable", err)
	}
}

// TestUnlowerableConstructsHandBack pins the section 30 contract: a construct
// whose slice has not landed returns a NotYetLowerable error, never a wrong Go
// type.
func TestUnlowerableConstructsHandBack(t *testing.T) {
	f := adapter.NewFake()
	num := f.Prim(adapter.TypeNumber)
	str := f.Prim(adapter.TypeString)

	cases := []struct {
		name string
		ty   *adapter.FakeType
	}{
		{"any", f.Any()},
		{"union", f.Union(num, str)},
		{"typeParameter", f.Prim(adapter.TypeTypeParameter)},
		{"intersection", f.Prim(adapter.TypeIntersection)},
		{"enum", f.Prim(adapter.TypeEnum)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := renderOf(t, tc.ty)
			var nyl *NotYetLowerable
			if !errors.As(err, &nyl) {
				t.Fatalf("RenderType(%s) err = %v, want a *NotYetLowerable", tc.name, err)
			}
		})
	}
}

// TestOptionalPropertyHandsBack pins that an optional property is not lowerable
// until the optional tagged type lands, so the whole object hands back.
func TestOptionalPropertyHandsBack(t *testing.T) {
	f := adapter.NewFake()
	num := f.Prim(adapter.TypeNumber)
	withOpt := f.Object(
		adapter.PropertyInfo{Name: "host", Type: f.Prim(adapter.TypeString)},
		adapter.PropertyInfo{Name: "port", Type: num, Optional: true},
	)
	_, _, err := renderOf(t, withOpt)
	var nyl *NotYetLowerable
	if !errors.As(err, &nyl) {
		t.Fatalf("RenderType(object with optional) err = %v, want a *NotYetLowerable", err)
	}
}

// TestNonIdentifierPropertyHandsBack pins that a property name Go cannot spell
// belongs in the side table, not a field, so the object hands back for now.
func TestNonIdentifierPropertyHandsBack(t *testing.T) {
	f := adapter.NewFake()
	weird := f.Object(adapter.PropertyInfo{Name: "has space", Type: f.Prim(adapter.TypeNumber)})
	_, _, err := renderOf(t, weird)
	var nyl *NotYetLowerable
	if !errors.As(err, &nyl) {
		t.Fatalf("RenderType(object with non-identifier key) err = %v, want a *NotYetLowerable", err)
	}
}

// checkGolden compares got against the named golden file, rewriting it under
// -update.
func checkGolden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if *update {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v (run with -update to create)", name, err)
	}
	if got != string(want) {
		t.Errorf("golden %s mismatch:\n--- got ---\n%s\n--- want ---\n%s", name, got, want)
	}
}
