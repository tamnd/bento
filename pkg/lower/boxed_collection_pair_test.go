package lower

import (
	"errors"
	"strings"
	"testing"
)

// TestBoxedCollPairMaterializesAsAnArray pins the choice this slice makes: an entry off a
// collection whose key, value or member slot the boxed-signature pass rewrote holds a box
// in at least one half, and a Go struct field has no room for a box, so the pair gives way
// rather than the value. It materializes as the two-element boxed array JavaScript says an
// entry is, and no interned tuple is emitted for it at all.
func TestBoxedCollPairMaterializesAsAnArray(t *testing.T) {
	const prelude = "type Row = { id: number; tag: string };\n" +
		"const raw = JSON.parse('{}') as Record<string, Row>;\n" +
		"const mm = new Map<string, Row>();\n" +
		"mm.set('a', raw['a']);\n" +
		"const bs = new Set<Row>();\n" +
		"bs.add(raw['a']);\n" +
		"const km = new Map<Row, number>();\n" +
		"km.set(raw['a'], 7);\n"
	cases := []struct {
		name string
		src  string
	}{
		{"map direct", "export function k(): number { let n = 0; for (const e of mm) { n += e[1].id; } return n; }\n"},
		{"map entries", "export function k(): number { let n = 0; for (const e of mm.entries()) { n += e[1].id; } return n; }\n"},
		{"set entries", "export function k(): number { let n = 0; for (const e of bs.entries()) { n += e[1].id; } return n; }\n"},
		{"boxed key", "export function k(): number { let n = 0; for (const e of km) { n += e[1]; } return n; }\n"},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			got := renderProgram(t, prelude+c.src)
			if !strings.Contains(got, "value.NewArrayValue([]value.Value{") {
				t.Errorf("the pair of a boxed collection did not materialize as a boxed array:\n%s", got)
			}
			if strings.Contains(got, "Tuple_") {
				t.Errorf("the pair of a boxed collection still built an interned tuple:\n%s", got)
			}
		})
	}
}

// TestBoxedCollPairWalksBothSnapshots pins how a Map's pair is built: the two snapshots
// are read once and walked together, the keys driving the range and the values indexed by
// the range position, so each turn's entry holds a key with its own value.
func TestBoxedCollPairWalksBothSnapshots(t *testing.T) {
	const src = "type Row = { id: number; tag: string };\n" +
		"const raw = JSON.parse('{}') as Record<string, Row>;\n" +
		"const mm = new Map<string, Row>();\n" +
		"mm.set('a', raw['a']);\n" +
		"export function k(): number { let n = 0; for (const e of mm) { n += e[1].id; } return n; }\n"
	got := renderProgram(t, src)
	for _, want := range []string{
		"_bt1 := _bt0.Keys()",
		"_bt2 := _bt0.Values()",
		"for _bt3, _bt4 := range _bt1 {",
		"e := value.NewArrayValue([]value.Value{value.StringValue(_bt4), _bt2[_bt3]})",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("a boxed map's pair did not print %q:\n%s", want, got)
		}
	}
}

// TestBoxedCollPairBoxesTheStaticHalf pins that only the slot the pass boxed hands back a
// box. The other half is still whatever Go value the checker named, so it boxes on its way
// into the pair through the same coercion a static argument to a boxed slot takes: a string
// key wraps as a string value, a number value as a number value.
func TestBoxedCollPairBoxesTheStaticHalf(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			"static key",
			"const mm = new Map<string, Row>();\nmm.set('a', raw['a']);\n" +
				"export function k(): number { let n = 0; for (const e of mm) { n += e[1].id; } return n; }\n",
			"value.NewArrayValue([]value.Value{value.StringValue(_bt4), _bt2[_bt3]})",
		},
		{
			"static value",
			"const km = new Map<Row, number>();\nkm.set(raw['a'], 7);\n" +
				"export function k(): number { let n = 0; for (const e of km) { n += e[1]; } return n; }\n",
			"value.NewArrayValue([]value.Value{_bt4, value.Number(_bt2[_bt3])})",
		},
	}
	const prelude = "type Row = { id: number; tag: string };\n" +
		"const raw = JSON.parse('{}') as Record<string, Row>;\n"
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			got := renderProgram(t, prelude+c.src)
			if !strings.Contains(got, c.want) {
				t.Errorf("the unboxed half of a pair did not print %q:\n%s", c.want, got)
			}
		})
	}
}

// TestBoxedSetPairIsTheMemberTwice pins the Set's entry: a Set's [key, value] is its member
// in both halves, so the one ranged member boxes into both slots off a single Members walk
// rather than two.
func TestBoxedSetPairIsTheMemberTwice(t *testing.T) {
	const src = "type Row = { id: number; tag: string };\n" +
		"const raw = JSON.parse('{}') as Record<string, Row>;\n" +
		"const bs = new Set<Row>();\n" +
		"bs.add(raw['a']);\n" +
		"export function k(): number { let n = 0; for (const e of bs.entries()) { n += e[1].id; } return n; }\n"
	got := renderProgram(t, src)
	if !strings.Contains(got, "for _, _bt0 := range bs.Members() {") {
		t.Errorf("a boxed set's entries did not walk one snapshot:\n%s", got)
	}
	if !strings.Contains(got, "e := value.NewArrayValue([]value.Value{_bt0, _bt0})") {
		t.Errorf("a boxed set's entry was not its member twice:\n%s", got)
	}
}

// TestBoxedCollPairCollectsThroughOneBuilder pins that every consumer of those entries goes
// through the same builder, the spread that splices them into a boxed array literal and the
// Array.from that collects them, the way collSnapshotSlice is the one reader for the
// collections the pass left alone.
func TestBoxedCollPairCollectsThroughOneBuilder(t *testing.T) {
	const prelude = "type Row = { id: number; tag: string };\n" +
		"const raw = JSON.parse('{}') as Record<string, Row>;\n" +
		"const mm = new Map<string, Row>();\n" +
		"mm.set('a', raw['a']);\n"
	cases := []struct{ name, src string }{
		{"spread", "export function k(): number { return [...mm].length; }\n"},
		{"spread entries", "export function k(): number { return [...mm.entries()].length; }\n"},
		{"array from", "export function k(): number { return Array.from(mm).length; }\n"},
		{"array from entries", "export function k(): number { return Array.from(mm.entries()).length; }\n"},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			got := renderProgram(t, prelude+c.src)
			for _, want := range []string{
				"value.NewArrayValue(append([]value.Value{}, func() []value.Value {",
				"make([]value.Value, len(",
				"value.NewArrayValue([]value.Value{value.StringValue(",
			} {
				if !strings.Contains(got, want) {
					t.Errorf("collecting a boxed collection's entries did not print %q:\n%s", want, got)
				}
			}
		})
	}
}

// TestBoxedPairDestructureAssignReadsThroughTheValueModel pins the assignment form:
// [k, v] = e over a bound pair reads each position through the runtime index rather than
// through the interned struct's E0 and E1 fields a value.Value has not got, and each read
// then lands in the slot its target already declared, a string coerced down and a boxed
// local taken as it stands.
func TestBoxedPairDestructureAssignReadsThroughTheValueModel(t *testing.T) {
	const src = "type Row = { id: number; tag: string };\n" +
		"const raw = JSON.parse('{}') as Record<string, Row>;\n" +
		"const mm = new Map<string, Row>();\n" +
		"mm.set('a', raw['a']);\n" +
		"export function k(): number {\n" +
		"  let k = '';\n" +
		"  let v: Row = raw['a'];\n" +
		"  for (const e of mm) { [k, v] = e; }\n" +
		"  return k.length + v.id;\n" +
		"}\n"
	got := renderProgram(t, src)
	if !strings.Contains(got, "k, v = value.ToString(e.GetIndex(0)), e.GetIndex(1)") {
		t.Errorf("an assignment destructure off a boxed pair did not read through the value model:\n%s", got)
	}
	if strings.Contains(got, ".E0") || strings.Contains(got, ".E1") {
		t.Errorf("an assignment destructure off a boxed pair read the interned struct's fields:\n%s", got)
	}
}

// TestBoxedPairDestructureAssignHandsBack pins the boundary that path leaves: a target
// whose own slot is a Go struct or another concrete shape has nowhere to put a box, so the
// statement hands back with a named reason rather than assigning one into it.
func TestBoxedPairDestructureAssignHandsBack(t *testing.T) {
	const src = "type Row = { id: number; tag: string };\n" +
		"const raw = JSON.parse('{}') as Record<string, Row>;\n" +
		"const mm = new Map<string, Row>();\n" +
		"mm.set('a', raw['a']);\n" +
		"export function k(): number {\n" +
		"  let k = '';\n" +
		"  let v: Row = { id: 0, tag: '' };\n" +
		"  for (const e of mm) { [k, v] = e; }\n" +
		"  return k.length + v.id;\n" +
		"}\n"
	prog := compile(t, src)
	r := NewRenderer(prog)
	_, err := r.RenderProgram(entryFile(t, prog))
	var nyl *NotYetLowerable
	if !errors.As(err, &nyl) {
		t.Fatalf("want a NotYetLowerable, got %v", err)
	}
	if !strings.Contains(nyl.Reason, "target whose own slot is not boxed") {
		t.Fatalf("want the unboxed-target reason, got %q", nyl.Reason)
	}
}
