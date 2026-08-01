package build

import "testing"

// TestAnAssertionBoundToANameKeepsItsBox is the capability this is for. An assertion
// read straight through, (e as Error).message, is erased and the box crosses it. An
// assertion bound to a name is the same assertion written over two lines, which is how
// a program that uses the value more than once has to write it:
//
//	const parsed = JSON.parse(s) as Config;
//
// That handed the build back, because the checker types the binding by the asserted
// type and the Go shape it names has no room for a box. The binding lands in a
// value.Value slot instead and is marked dynamic, so every later read off the name
// dispatches the way a read straight off the assertion does.
//
// This builds a real binary and holds its whole output against Node v24.18.0.
func TestAnAssertionBoundToANameKeepsItsBox(t *testing.T) {
	got := buildAndRunFile(t, "main.ts",
		"type Config = { name: string; port: number };\n"+
			"const parsed = JSON.parse('{\"name\":\"a\",\"port\":8080}') as Config;\n"+
			"console.log(parsed.name, parsed.port);\n"+
			"const u: unknown = [1, 2, 3];\n"+
			"const arr = u as number[];\n"+
			"console.log(arr.length, arr[0]);\n"+
			"const o: any = { n: 2 };\n"+
			"const s = o as { n: number };\n"+
			"console.log(s.n);\n"+
			"function f(): void { throw new Error('bad'); }\n"+
			"try { f(); } catch (e) {\n"+
			"  const err = e as Error;\n"+
			"  console.log(err.message, err.name);\n"+
			"}\n")
	want := "a 8080\n3 1\n2\nbad Error\n"
	if got != want {
		t.Errorf("an assertion bound to a name read differently than Node:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestReadsOffAnAssertionBoundToANameDispatchDynamically is the other half, and the
// half that would have shipped wrong Go rather than a hand-back. The checker still
// types the binding by the asserted shape, so a for...of over cfg.list ranged .Elems
// on a value.Value and cfg.list.join('-') called .Join on one, neither of which is a
// Go program. The iteration, the method call, and the optional chain each ask the
// question at run time now, the way every other read off a box does.
//
// The last line is the same chain with no binding in it at all, which note 379 left
// broken the same way: the assertion is erased, so the read off it and the method on
// that read are both off a box whatever the checker calls them.
//
// Held against what Node v24.18.0 prints, including the inspector and JSON forms,
// which is where a dropped or renamed property would show.
func TestReadsOffAnAssertionBoundToANameDispatchDynamically(t *testing.T) {
	got := buildAndRunFile(t, "main.ts",
		"type Cfg = { name: string; flag: boolean; a: { b: number }; list: number[] };\n"+
			"const cfg = JSON.parse('{\"name\":\"n\",\"flag\":true,\"a\":{\"b\":2},\"list\":[1,2]}') as Cfg;\n"+
			"console.log(cfg.a.b);\n"+
			"if (cfg.flag) console.log(`hi ${cfg.name}`);\n"+
			"for (const x of cfg.list) console.log(x);\n"+
			"cfg.list.push(4);\n"+
			"console.log(cfg.list.join('-'));\n"+
			"console.log(Object.keys(cfg));\n"+
			"console.log(JSON.stringify(cfg));\n"+
			"console.log(cfg);\n"+
			"console.log((JSON.parse('{\"list\":[7,8]}') as Cfg).list.join('+'));\n")
	want := "2\n" +
		"hi n\n" +
		"1\n2\n" +
		"1-2-4\n" +
		"[ 'name', 'flag', 'a', 'list' ]\n" +
		"{\"name\":\"n\",\"flag\":true,\"a\":{\"b\":2},\"list\":[1,2,4]}\n" +
		"{ name: 'n', flag: true, a: { b: 2 }, list: [ 1, 2, 4 ] }\n" +
		"7+8\n"
	if got != want {
		t.Errorf("reads off a name bound to an assertion read differently than Node:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestDestructuringAndOptionalChainOffAnAssertion covers the two shapes that bind or
// read past the box rather than off it. A destructuring declaration binds each name to
// a box, so every name it introduces is marked dynamic, not only the rest it gathers:
// the checker calls them number and string, and a later read would take them for Go
// values they are not. An optional chain over a boxed receiver asks the nullish
// question at run time, since the box carries null and undefined among its kinds,
// where the option path would have mapped over a value.Value that is not an option.
//
// Held against what Node v24.18.0 prints.
func TestDestructuringAndOptionalChainOffAnAssertion(t *testing.T) {
	got := buildAndRunFile(t, "main.ts",
		"type P = { a: number; b: string };\n"+
			"const { a, b } = JSON.parse('{\"a\":1,\"b\":\"x\"}') as P;\n"+
			"console.log(a, b);\n"+
			"type A = { a?: { b: number } };\n"+
			"const v = JSON.parse('{}') as A;\n"+
			"console.log(v.a?.b);\n"+
			"const w = JSON.parse('{\"a\":{\"b\":5}}') as A;\n"+
			"console.log(w.a?.b);\n")
	want := "1 x\nundefined\n5\n"
	if got != want {
		t.Errorf("destructuring or an optional chain off an assertion read differently than Node:\ngot:\n%s\nwant:\n%s", got, want)
	}
}
