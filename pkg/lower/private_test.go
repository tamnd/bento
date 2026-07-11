package lower

import (
	"strings"
	"testing"
)

// TestPrivateFieldEmitsPrefixedField pins that a #field lowers to an unexported
// p_-prefixed struct field carrying the json:"-" tag, since a private field is
// invisible to JSON.stringify.
func TestPrivateFieldEmitsPrefixedField(t *testing.T) {
	const src = `class C {
  #x: number = 1;
  pub: number = 2;
}
new C();
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "p_x float64 `json:\"-\"`") {
		t.Errorf("private field did not lower to a p_-prefixed json:\"-\" field:\n%s", source)
	}
	if !strings.Contains(source, "Pub float64 `json:\"pub\"`") {
		t.Errorf("public field lost its exported spelling:\n%s", source)
	}
}

// TestPrivateMethodEmitsPrefixedMethod pins that a #method lowers to an
// unexported p_-prefixed method, coexisting with a public method of the same
// base name spelled M.
func TestPrivateMethodEmitsPrefixedMethod(t *testing.T) {
	const src = `class C {
  m(): number { return 1; }
  #m(): number { return 2; }
}
new C();
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "func (c *C) M() float64") {
		t.Errorf("public method not spelled M:\n%s", source)
	}
	if !strings.Contains(source, "func (c *C) p_m() float64") {
		t.Errorf("private method not spelled p_m:\n%s", source)
	}
}

// TestPrivateMembersRun runs the emitted Go and pins that a private field read
// and written through this, a private method called through this, and a public
// method of the same base name all resolve to their distinct Go spellings.
func TestPrivateMembersRun(t *testing.T) {
	const src = `class C {
  #x: number = 1;
  m(): number { return 10; }
  #m(): number { return 20; }
  inc(): void { this.#x = this.#x + 1; }
  read(): number { return this.#x + this.m() + this.#m(); }
}
const c = new C();
c.inc();
console.log(String(c.read()));
`
	got := runProgramGo(t, src)
	if got != "32\n" {
		t.Errorf("private members ran wrong\n got: %q\nwant: %q", got, "32\n")
	}
}

// TestPrivateNameCoexistsWithPublic pins that #m and m are distinct members: a
// class may declare both because their Go spellings differ.
func TestPrivateNameCoexistsWithPublic(t *testing.T) {
	const src = `class C {
  #n: number = 5;
  n: number = 7;
  sum(): number { return this.#n + this.n; }
}
console.log(String(new C().sum()));
`
	got := runProgramGo(t, src)
	if got != "12\n" {
		t.Errorf("private and public same-base names collided\n got: %q\nwant: %q", got, "12\n")
	}
}

// TestPrivateHandsBack pins the boundary: each private construct outside the
// #method and #field subset hands back with its own honest reason.
func TestPrivateHandsBack(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			"brandCheck",
			"class C { #m(): number { return 1; } has(o: any): boolean { return #m in o; } }\nnew C();\n",
			"a private brand check (#m in obj) is a later slice",
		},
		{
			"privateStaticGetter",
			"class C { static get #x(): number { return 1; } }\nnew C();\n",
			"a private get accessor is a later slice",
		},
		{
			"privateStaticSetter",
			"class C { static #v = 0; static set #x(v: number) { C.#v = v; } }\nnew C();\n",
			"a private set accessor is a later slice",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			reason := renderProgramHandBack(t, tc.src)
			if !strings.Contains(reason, tc.want) {
				t.Errorf("hand-back reason %q does not name %q", reason, tc.want)
			}
		})
	}
}
