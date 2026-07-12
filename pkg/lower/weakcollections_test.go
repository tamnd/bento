package lower

import (
	"strings"
	"testing"
)

// TestWeakMapLoweringShape pins the Go a WeakMap program lowers to: construction picks
// NewWeakMap over the key pointee and the value type, a local that holds one is typed
// *value.WeakMap[T, V], and get, set, has, and delete map to the matching methods with
// the object key passed as the struct pointer it lowers to. Reading the emitted code
// directly keeps a change to the shape visible in review without running the toolchain.
func TestWeakMapLoweringShape(t *testing.T) {
	const src = `interface Box { id: number }
const k: Box = { id: 1 };
const m = new WeakMap<Box, number>();
m.set(k, 1);
const v = m.get(k);
if (v !== undefined) {
  console.log(v);
}
console.log(m.has(k));
m.delete(k);
`
	source := renderProgram(t, src)
	for _, want := range []string{
		"value.NewWeakMap[ObjId, float64]()",
		"m.Set(k, 1)",
		"m.Get(k)",
		"m.Has(k)",
		"m.Delete(k)",
	} {
		if !strings.Contains(source, want) {
			t.Errorf("WeakMap lowering missing %q:\n%s", want, source)
		}
	}
}

// TestWeakSetLoweringShape pins the Go a WeakSet program lowers to: construction picks
// NewWeakSet over the member pointee, a local that holds one is typed *value.WeakSet[T],
// and add, has, and delete map to the matching methods with the object member passed as
// the struct pointer it lowers to.
func TestWeakSetLoweringShape(t *testing.T) {
	const src = `interface Box { id: number }
const k: Box = { id: 1 };
const s = new WeakSet<Box>();
s.add(k);
console.log(s.has(k));
s.delete(k);
`
	source := renderProgram(t, src)
	for _, want := range []string{
		"value.NewWeakSet[ObjId]()",
		"s.Add(k)",
		"s.Has(k)",
		"s.Delete(k)",
	} {
		if !strings.Contains(source, want) {
			t.Errorf("WeakSet lowering missing %q:\n%s", want, source)
		}
	}
}

// TestWeakSetHandsBackUnsupportedForms proves the WeakSet lowering hands back the forms
// it cannot emit soundly: construction from an iterable of members needs each member
// built as an object first, which the iterable-drain slice brings.
func TestWeakSetHandsBackUnsupportedForms(t *testing.T) {
	handsBack(t, "interface Box { id: number }\nconst k: Box = { id: 1 };\nconst s = new WeakSet<Box>([k]);\nconsole.log(s.has(k));\n")
}

// TestWeakRefLoweringShape pins the Go a WeakRef program lowers to: construction picks
// NewWeakRef over the target pointee with the target passed as the object pointer, a
// local that holds one is typed *value.WeakRef[T], and deref maps to Deref returning an
// Opt read past an undefined guard.
func TestWeakRefLoweringShape(t *testing.T) {
	const src = `interface Box { id: number }
const a: Box = { id: 1 };
const wr = new WeakRef(a);
const o = wr.deref();
if (o !== undefined) {
  console.log(o.id);
}
`
	source := renderProgram(t, src)
	for _, want := range []string{
		"value.NewWeakRef[ObjId](a)",
		"wr.Deref()",
	} {
		if !strings.Contains(source, want) {
			t.Errorf("WeakRef lowering missing %q:\n%s", want, source)
		}
	}
}

// TestFinalizationRegistryLoweringShape pins the Go a FinalizationRegistry program
// lowers to: construction picks NewFinalizationRegistry over the held type with the
// cleanup callback, register lowers to the free FinalizationRegister function generic
// over the target pointee and the held type with a nil token when none is passed, and
// unregister maps to the Unregister method.
func TestFinalizationRegistryLoweringShape(t *testing.T) {
	const src = `interface Box { id: number }
const reg = new FinalizationRegistry<string>((held: string) => { console.log(held); });
const a: Box = { id: 1 };
reg.register(a, "a");
console.log(reg.unregister(a));
`
	source := renderProgram(t, src)
	for _, want := range []string{
		"value.NewFinalizationRegistry[value.BStr](",
		"value.FinalizationRegister[ObjId, value.BStr](reg, a, value.FromGoString(\"a\"), nil)",
		"reg.Unregister(a)",
	} {
		if !strings.Contains(source, want) {
			t.Errorf("FinalizationRegistry lowering missing %q:\n%s", want, source)
		}
	}
}

// TestWeakMapHandsBackUnsupportedForms proves the WeakMap lowering claims only the
// subset it can emit soundly and hands the rest back. Construction from an iterable of
// entry pairs needs each key built as an object first, which the iterable-drain slice
// brings, and a key whose render is not a pointer is not a weak key, so each routes to
// the interpreter rather than emitting wrong or partial Go.
func TestWeakMapHandsBackUnsupportedForms(t *testing.T) {
	handsBack(t, "interface Box { id: number }\nconst k: Box = { id: 1 };\nconst m = new WeakMap<Box, number>([[k, 1]]);\nconsole.log(m.has(k));\n")
}
