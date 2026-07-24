package build

import "testing"

// TestStructuredCloneDeepCopiesObject pins slice G1.3: structuredClone(value) returns a
// deep copy, a fresh object whose nested object is itself a copy, so a write through the
// clone does not reach the original. The source is built with JSON.parse so it is a live
// dynamic object graph, the domain structuredClone walks. The body clones it, mutates the
// clone's nested field, and prints both sides; Node prints the original unchanged, then
// the clone's new value, then false for the nested identity.
func TestStructuredCloneDeepCopiesObject(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"const src = JSON.parse('{\"a\":1,\"nested\":{\"b\":2}}');\n"+
			"const copy = structuredClone(src);\n"+
			"copy.nested.b = 99;\n"+
			"console.log(src.nested.b);\n"+
			"console.log(copy.nested.b);\n"+
			"console.log(src.nested === copy.nested);\n")
	if want := "2\n99\nfalse\n"; got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestStructuredCloneArrayPreservesHole pins that an array clones its elements and keeps
// a hole a hole rather than filling it. The source is a dynamic array with its middle
// index deleted, so index 1 is a genuine hole; the clone must carry the same length and
// the same absent index. Node prints the length, false for the still-absent index, then
// the two present values.
func TestStructuredCloneArrayPreservesHole(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"const src = JSON.parse('[1,2,3]');\n"+
			"delete src[1];\n"+
			"const copy = structuredClone(src);\n"+
			"console.log(copy.length);\n"+
			"console.log(1 in copy);\n"+
			"console.log(copy[0]);\n"+
			"console.log(copy[2]);\n")
	if want := "3\nfalse\n1\n3\n"; got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestStructuredClonePreservesSharedAndCyclicRefs pins that the clone preserves the
// input graph's sharing: an object referenced twice is one object twice in the clone,
// and a cycle clones to a cycle rather than looping forever. The graph is built on
// dynamic objects so the two references are genuinely one object; holder.self makes it
// cyclic. Node prints true three times: the two clone paths reach one object, that
// object is not the original, and the self-cycle survived.
func TestStructuredClonePreservesSharedAndCyclicRefs(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"const shared = JSON.parse('{\"v\":1}');\n"+
			"const holder = JSON.parse('{}');\n"+
			"holder.x = shared;\n"+
			"holder.y = shared;\n"+
			"holder.self = holder;\n"+
			"const copy = structuredClone(holder);\n"+
			"console.log(copy.x === copy.y);\n"+
			"console.log(copy.x !== shared);\n"+
			"console.log(copy.self === copy);\n")
	if want := "true\ntrue\ntrue\n"; got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestStructuredCloneThrowsOnFunction pins the honest-throw rule: a value the
// structured-clone algorithm cannot carry, a function, throws rather than returning a
// lossy copy. Node throws a DataCloneError DOMException here; bento reports the same
// fact through its own error type, so this is pinned by bento's message rather than
// compared against Node. The body clones a function inside a try and prints the message.
func TestStructuredCloneThrowsOnFunction(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"try { structuredClone(() => {}); } catch (e) { console.log(e.message); }\n")
	if want := "a function could not be cloned\n"; got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}
