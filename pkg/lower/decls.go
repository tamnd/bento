package lower

import (
	"go/format"
	"sort"
	"strings"

	"github.com/tamnd/bento/pkg/frontend"
)

// This file owns the set of generated Go declarations a render pass produces,
// today the structs that object shapes lower to (05_type_lowering.md section
// 12). Two rules from the spec drive its design. First, a distinct structural
// shape lowers to exactly one Go struct, interned so that every object literal
// with the same fields shares the type and structural assignability becomes Go
// assignability (section 12). Second, the generated name is derived
// deterministically from the shape, so the same shape yields the same name
// across compilation units and builds (section 29).

// Decl is one generated Go declaration: a name and its gofmt-clean source.
type Decl struct {
	Name   string
	Source string
}

// declSet accumulates generated declarations during a render pass and interns
// object structs by structural identity. It keys on frontend.Type.Identity,
// which is the checker's structural identity surfaced through the frontend: two
// object types with the same shape share an identity, so they share a struct.
// The identity is only stable within one program, which is why a Renderer, and
// so a declSet, is scoped to one program.
type declSet struct {
	// nameByIdentity maps a type's structural identity to the struct name it was
	// assigned, so a second render of the same shape reuses the first name and
	// so a recursive shape can refer to its own name mid-construction.
	nameByIdentity map[int]string
	// source holds each generated declaration's gofmt-clean text, keyed by name.
	// A name maps to an empty string while its struct is still being built,
	// which is how a self-referential shape is broken: the name is reserved
	// before its fields are rendered.
	source map[string]string
	// order is the names in first-seen order, so emission is stable.
	order []string
	// used records every assigned name so a second shape that derives the same
	// base name gets a deterministic numbered suffix instead of colliding.
	used map[string]bool
}

func newDeclSet() *declSet {
	return &declSet{
		nameByIdentity: map[int]string{},
		source:         map[string]string{},
		used:           map[string]bool{},
	}
}

// internStruct returns the Go struct name for an object type, generating the
// struct declaration the first time it sees the shape and reusing the name after
// that. It reserves the name before rendering the fields so a field whose type
// refers back to this same object (a recursive shape) resolves to the reserved
// name instead of recursing forever.
func (d *declSet) internStruct(r *Renderer, t frontend.Type) (string, error) {
	id := t.Identity()
	if name, ok := d.nameByIdentity[id]; ok {
		return name, nil
	}

	props := r.prog.Properties(t)
	name := d.reserve(structBaseName(props))
	d.nameByIdentity[id] = name

	body, err := renderStructBody(r, name, props)
	if err != nil {
		// Roll the reservation back so a later, lowerable use of a shape that
		// happens to share this base name is not pushed to a suffix by a failure.
		delete(d.nameByIdentity, id)
		delete(d.used, name)
		d.order = d.order[:len(d.order)-1]
		return "", err
	}
	d.source[name] = body
	return name, nil
}

// reserve picks a unique name from a base, appending _2, _3, and so on when the
// base is already taken by a different shape, and records it as used and in
// emission order.
func (d *declSet) reserve(base string) string {
	name := base
	for n := 2; d.used[name]; n++ {
		name = base + "_" + itoa(n)
	}
	d.used[name] = true
	d.order = append(d.order, name)
	return name
}

// emit returns the declarations in first-seen order.
func (d *declSet) emit() []Decl {
	out := make([]Decl, 0, len(d.order))
	for _, name := range d.order {
		out = append(out, Decl{Name: name, Source: d.source[name]})
	}
	return out
}

// renderStructBody renders one struct declaration: a field per property, in the
// source declaration order the frontend preserves so layout is stable (section
// 29). The result is run through go/format so it is gofmt-clean, which section 2
// requires so a developer reading a stack trace sees legible Go.
func renderStructBody(r *Renderer, name string, props []frontend.Property) (string, error) {
	var b strings.Builder
	b.WriteString("type ")
	b.WriteString(name)
	b.WriteString(" struct {\n")
	for _, p := range props {
		if p.Optional {
			// An optional property is T | undefined plus a presence bit, which
			// lowers to the section 9 optional type. That type is a later slice,
			// so a shape with an optional property is not lowerable yet.
			return "", &NotYetLowerable{Flags: p.Type.Flags, Reason: "optional property needs the optional tagged type, a later slice"}
		}
		field, ok := exportedField(p.Name)
		if !ok {
			// A non-identifier property name (a space, a numeric key) belongs in
			// the object's symbol/string side table, not a struct field, which
			// is a later slice.
			return "", &NotYetLowerable{Flags: p.Type.Flags, Reason: "non-identifier property name belongs in the object side table"}
		}
		goType, err := r.RenderType(p.Type)
		if err != nil {
			return "", err
		}
		b.WriteString("\t")
		b.WriteString(field)
		b.WriteString(" ")
		b.WriteString(goType)
		b.WriteString("\n")
	}
	b.WriteString("}\n")

	return formatDecl(b.String())
}

// formatDecl runs one generated top-level declaration through go/format so it is
// gofmt-clean, which section 2 requires so a developer reading a stack trace
// sees legible Go. A format failure means the emitted text is not valid Go,
// which is a lowering bug rather than a source-driven boundary, so it surfaces
// as a NotYetLowerable naming the offending source rather than crashing.
func formatDecl(src string) (string, error) {
	formatted, err := format.Source([]byte(src))
	if err != nil {
		return "", &NotYetLowerable{Reason: "generated declaration did not format: " + err.Error()}
	}
	return string(formatted), nil
}

// structBaseName derives the deterministic base name of an object struct from
// its property names, matching the Obj + capitalized-property-names convention
// the spec uses in its examples (ObjName, ObjIncGet). A property whose name is
// not a Go identifier is skipped for the purpose of the name, because it does
// not become a field; renderStructBody rejects such a shape anyway, so the name
// is only ever used for shapes made entirely of identifier fields. An empty
// object is ObjEmpty so it still has a legal, stable name.
func structBaseName(props []frontend.Property) string {
	names := make([]string, 0, len(props))
	for _, p := range props {
		if field, ok := exportedField(p.Name); ok {
			names = append(names, field)
		}
	}
	if len(names) == 0 {
		return "ObjEmpty"
	}
	// Sort so the name is independent of property order, since two shapes that
	// differ only in declaration order are the same structural type and must
	// share a name.
	sort.Strings(names)
	return "Obj" + strings.Join(names, "")
}

// itoa is a tiny non-allocating-enough integer to string for suffixing, kept
// local so the file has no strconv dependency for one use.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
