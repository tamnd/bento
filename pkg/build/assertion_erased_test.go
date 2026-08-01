package build

import "testing"

// TestAnAssertionOverABoxedValueIsErased is the capability this is for. TypeScript types
// a caught binding unknown and will not let a program read .message off it, so essentially
// every catch block in TypeScript is written (e as Error).message. That handed the build
// back twice over: the Error type would not intern, because the es2022 lib declares
// cause?: unknown on it, and the assertion itself asked for a coercion from a boxed value
// into the Go struct that type interns to, which no dynamic value can become.
//
// An assertion is erased at run time. It changes what the checker believes and nothing
// about the value, so the box crosses it unchanged and the reads off it dispatch the way
// reads off any other box do. The same shape covers the other everyday assertions over a
// dynamic value: a JSON.parse result read at an asserted shape, an unknown read as an
// array, an any read as an object.
//
// This builds a real binary and holds its whole output against what Node v24.18.0 prints.
func TestAnAssertionOverABoxedValueIsErased(t *testing.T) {
	got := buildAndRunFile(t, "main.ts",
		"function f(): void { throw new Error('bad'); }\n"+
			"try { f(); } catch (e) { console.log((e as Error).message); }\n"+
			"try { f(); } catch (e) { console.log((e as Error).name); }\n"+
			"console.log((JSON.parse('{\"a\":1,\"b\":\"x\"}') as { a: number; b: string }).a);\n"+
			"const u: unknown = [1, 2, 3];\n"+
			"console.log((u as number[]).length);\n"+
			"console.log((u as number[])[1]);\n"+
			"const o: any = { n: 2 };\n"+
			"console.log((o as { n: number }).n);\n")
	want := "bad\nError\n1\n3\n2\n2\n"
	if got != want {
		t.Errorf("an assertion over a boxed value read differently than Node:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestAnOptionalDynamicPropertyReadsAsUndefined covers the shape that made the Error type
// unlowerable: an optional property typed unknown, which is what the es2022 lib declares
// cause as. It takes a bare value.Value rather than an option, since undefined is a kind
// the box already carries, and the point of that is what the property does at run time.
// An omitted one reads undefined, is skipped by JSON.stringify and by the inspector the
// way an absent key is, and a present one reads as the value it holds.
//
// Held against what Node v24.18.0 prints.
func TestAnOptionalDynamicPropertyReadsAsUndefined(t *testing.T) {
	got := buildAndRunFile(t, "main.ts",
		"type Bag = { a: number; b?: unknown };\n"+
			"const g: Bag = { a: 1 };\n"+
			"console.log(JSON.stringify(g));\n"+
			"console.log(g);\n"+
			"console.log(g.b);\n"+
			"const h: Bag = { a: 1, b: 'x' };\n"+
			"console.log(JSON.stringify(h));\n"+
			"console.log(h.b);\n")
	want := "{\"a\":1}\n{ a: 1 }\nundefined\n{\"a\":1,\"b\":\"x\"}\nx\n"
	if got != want {
		t.Errorf("an optional dynamic property read differently than Node:\ngot:\n%s\nwant:\n%s", got, want)
	}
}
