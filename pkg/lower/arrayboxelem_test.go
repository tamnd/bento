package lower

import "testing"

// TestDynamicArrayElementBoxes pins that a concrete element spliced into an any[]
// is boxed to value.Value, the store an any[] holds, rather than left as its bare
// Go type the boxed slice cannot take. The self-referential spread widens the
// element to any, so the number the splice adds must box.
func TestDynamicArrayElementBoxes(t *testing.T) {
	skipIfShort(t)
	const src = `let additional: any[] = [];
for (const subcomponent of [1, 2, 3]) {
    additional = [...additional, subcomponent];
}
console.log(additional.length);
console.log(additional[2]);`
	got := runProgramGo(t, src)
	want := "3\n3\n"
	if got != want {
		t.Fatalf("dynamic array element boxing run mismatch:\n got %q\nwant %q", got, want)
	}
}
