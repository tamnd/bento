package lower

import (
	"strings"
	"testing"
)

// An optional property whose declared type is bare any lowers to a value.Value
// field, with undefined a first-class value in the box. The checker folds
// any | undefined back to any, so the field carries no union flag and takes the
// dynamic slot the way a dynamic optional parameter does, rather than handing back
// on the tagged-sum path a wider optional union needs.

// TestOptionalAnyPropertyLowers proves the struct field is the dynamic box and no
// value.Opt or tagged sum is emitted for an any-typed optional property.
func TestOptionalAnyPropertyLowers(t *testing.T) {
	const src = `const d: { value?: any } = { value: 42 };
console.log(d.value);
`
	source := renderProgramTolerant(t, src)
	if strings.Contains(source, "value.Opt[") {
		t.Fatalf("optional any property lowered to value.Opt rather than the dynamic box:\n%s", source)
	}
}

// TestOptionalAnyPresentRuns builds and runs a present any-typed optional member
// against the Node oracle.
func TestOptionalAnyPresentRuns(t *testing.T) {
	skipIfShort(t)
	const src = `const d: { value?: any } = { value: 42 };
console.log(d.value);
`
	if got, want := runProgramGoTolerant(t, src), "42\n"; got != want {
		t.Fatalf("present optional-any member printed %q, want %q", got, want)
	}
}

// TestOptionalAnyOmittedRuns builds and runs an omitted any-typed optional member,
// which reads back undefined the way the language leaves an absent property.
func TestOptionalAnyOmittedRuns(t *testing.T) {
	skipIfShort(t)
	const src = `const d: { value?: any } = {};
console.log(d.value);
`
	if got, want := runProgramGoTolerant(t, src), "undefined\n"; got != want {
		t.Fatalf("omitted optional-any member printed %q, want %q", got, want)
	}
}

// TestOptionalAnyExplicitUndefinedRuns builds and runs an any-typed optional member
// set explicitly to undefined, which reads back undefined the same as an omitted one.
func TestOptionalAnyExplicitUndefinedRuns(t *testing.T) {
	skipIfShort(t)
	const src = `const d: { value?: any } = { value: undefined };
console.log(d.value);
`
	if got, want := runProgramGoTolerant(t, src), "undefined\n"; got != want {
		t.Fatalf("explicit-undefined optional-any member printed %q, want %q", got, want)
	}
}
