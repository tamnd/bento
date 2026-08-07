package lower

import (
	"strings"
	"testing"
)

// Optional element access a?.[i] is the bracketed spelling of the optional chain. The
// receiver decides the lowering: a box dispatches at run time, an annotated optional
// maps over its inner value, and a receiver the checker already settled at present or
// at nullish skips the test entirely. These pin which shape gets which, since all of
// them print the same thing and only the emitted Go says which path ran.

// TestABoxedReceiverIndexesThroughTheHelper is the everyday shape, and the one the
// compat suite stops on: a regexp match is a box that may be null, and reading group 1
// off it has to answer undefined rather than dereference.
func TestABoxedReceiverIndexesThroughTheHelper(t *testing.T) {
	got := renderProgram(t, `const m = /a(b)/.exec("xaby");
console.log(m?.[1]);`)
	if !strings.Contains(got, "value.OptionalIndex(m, 1)") {
		t.Errorf("did not emit value.OptionalIndex(m, 1):\n%s", got)
	}
}

// TestABoxedReceiverPicksTheHelperByIndexType pins the three key kinds apart. A number
// is an index, a string is a member name, and anything decided at run time goes through
// the element lookup that sorts the two out itself.
func TestABoxedReceiverPicksTheHelperByIndexType(t *testing.T) {
	cases := []struct {
		name string
		idx  string
		want string
	}{
		{"a number", `0`, "value.OptionalIndex("},
		{"a string binding", `key`, "value.OptionalMember("},
		{"a dynamic binding", `dyn`, "value.OptionalElem("},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := renderProgram(t, `const o: any = { m: { a: 5 } };
const key: string = "a";
const dyn: any = "a";
console.log(o.m?.[`+tc.idx+`]);`)
			if !strings.Contains(got, tc.want) {
				t.Errorf("did not emit %s:\n%s", tc.want, got)
			}
		})
	}
}

// TestAnEffectfulIndexIsNotEvaluatedOnANullishReceiver is the reason the flat helper is
// not the only spelling. A Go call evaluates its arguments before it runs, so passing
// the index to a helper would call idx() even when the receiver is nullish, which the
// language says never happens. An index that can run code moves inside the test.
func TestAnEffectfulIndexIsNotEvaluatedOnANullishReceiver(t *testing.T) {
	got := renderProgram(t, `const o: any = undefined;
function idx(): number { return 0; }
console.log(o?.[idx()]);`)
	if strings.Contains(got, "value.OptionalIndex(") {
		t.Errorf("passed an effectful index to the flat helper, which evaluates it:\n%s", got)
	}
	if !strings.Contains(got, "v.IsNullish()") || !strings.Contains(got, "v.GetIndex(Idx())") {
		t.Errorf("did not guard the index behind a nullish test:\n%s", got)
	}
}

// TestAnAnnotatedOptionalMapsOverItsInner covers the receiver that is not a box: an
// Opt[T] has its own present test, so the read is the inner value's own read lifted
// through OptMap rather than a run-time dispatch.
func TestAnAnnotatedOptionalMapsOverItsInner(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{"an array", `function f(a: number[] | undefined, i: number): number | undefined { return a?.[i]; }`, "v.At(i)"},
		{"a string", `function f(s: string | undefined, i: number): string | undefined { return s?.[i]; }`, "v.CharAt(i)"},
		{"a tuple", `function f(t: [number, string] | undefined): string | undefined { return t?.[1]; }`, "return v.E1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := renderProgram(t, tc.src+"\n")
			if !strings.Contains(got, "value.OptMap(") || !strings.Contains(got, tc.want) {
				t.Errorf("did not map %s through OptMap with %s:\n%s", tc.name, tc.want, got)
			}
		})
	}
}

// TestAConstantStringKeyOnAnOptionalReadsTheField pins that o?.["k"] is o?.k. The key is
// a literal the emitter can read, so the shape's field is known and the read is the
// dotted one, which keeps the two spellings emitting the same Go.
func TestAConstantStringKeyOnAnOptionalReadsTheField(t *testing.T) {
	got := renderProgram(t, `function f(o: { k: number } | undefined): number | undefined { return o?.["k"]; }
`)
	if !strings.Contains(got, ".K") {
		t.Errorf("a constant key did not read the field:\n%s", got)
	}
}

// TestAReceiverThatIsNeverNullishReadsStraightThrough is the narrowing case. The checker
// proves the receiver present, so the ?. has nothing to test and the read is the plain
// one. Emitting a chain here would wrap a value in an optional the program never had.
func TestAReceiverThatIsNeverNullishReadsStraightThrough(t *testing.T) {
	got := renderProgram(t, `const arr: number[] | undefined = [1, 2];
console.log(arr?.[1]);`)
	if strings.Contains(got, "value.Optional") || strings.Contains(got, "value.OptMap(") {
		t.Errorf("tested a receiver the checker proved present:\n%s", got)
	}
	if !strings.Contains(got, ".At(1)") {
		t.Errorf("did not read straight through:\n%s", got)
	}
}

// TestAReceiverThatIsSettledNullishFolds is the other half of the same proof. The
// receiver is undefined and nothing else, so the whole access is undefined and there is
// no index read to emit at all.
func TestAReceiverThatIsSettledNullishFolds(t *testing.T) {
	got := renderProgram(t, `const gone: number[] | undefined = undefined;
console.log(gone?.[1]);`)
	if !strings.Contains(got, "value.MissingProperty(") {
		t.Errorf("did not fold a settled nullish receiver:\n%s", got)
	}
	if strings.Contains(got, ".At(1)") {
		t.Errorf("emitted an index read on a receiver that is never there:\n%s", got)
	}
}

// TestAnOptionalCallHandsBackByName pins the neighbouring form the bracket work does not
// close. f?.(x) carries the same ?. token, and without a case of its own the token lowers
// as if it were an expression and the reader is told a token kind is a later slice. Name
// the form instead.
func TestAnOptionalCallHandsBackByName(t *testing.T) {
	reason := renderProgramHandBack(t, `const f: any = (n: number) => n;
console.log(f?.(1));
`)
	if !strings.Contains(reason, "an optional call f?.() is a later slice") {
		t.Errorf("hand back said %q, want it to name the optional call", reason)
	}
}

// TestAnOptionalWithNoIndexReadHandsBack keeps the annotated path's limit honest. A
// computed string key on an annotated optional names no field the emitter can read and
// no numeric index it can lift, so it stops rather than invent one.
func TestAnOptionalWithNoIndexReadHandsBack(t *testing.T) {
	reason := renderProgramHandBack(t, `export function f(o: { [k: string]: number } | undefined, k: string): number | undefined { return o?.[k]; }
`)
	if !strings.Contains(reason, "later slice") {
		t.Errorf("hand back said %q, want a later-slice reason", reason)
	}
}
