package build

import "testing"

// TestADeclaredSignatureTakesABox covers the shape note 384 ended on. A program walks
// parsed JSON and hands a piece of it to a function it declared:
//
//	function pick(r: Row): number { return r.id }
//	pick(Object.values(m)[0]);
//
// The walk hands back a box and the parameter's Go slot was the struct the checker
// interned for Row, which a box has no fields for. The signature gives way rather than
// the value: the parameter takes a value.Value slot, its body reads the name through the
// value model, and every call site boxes what it passes.
//
// That has to be one decision about the function taken from all its call sites at once,
// because a Go function has one signature, so the static literal call in the first line
// below boxes on the way in and lands in the same slot the boxed call does.
//
// Held against what Node v24.18.0 prints, one program so the ordering is pinned too.
func TestADeclaredSignatureTakesABox(t *testing.T) {
	got := buildAndRunFile(t, "main.ts",
		"type Row = { id: number; tag: string };\n"+
			"const m = JSON.parse('{\"a\":{\"id\":1,\"tag\":\"x\"},\"b\":{\"id\":2,\"tag\":\"y\"}}') as Record<string, Row>;\n"+
			"function pick(r: Row): number { return r.id; }\n"+
			"const label = (r: Row): string => `${r.tag}#${r.id}`;\n"+
			"function join(a: Row, b: Row): string { return a.tag + b.tag; }\n"+
			"function inner(r: Row): string { return r.tag; }\n"+
			"function outer(r: Row): string { return inner(r) + '!'; }\n"+
			"function count(rows: Row[]): number { let s = 0; for (const r of rows) { s += r.id; } return s; }\n"+
			"function spell({ id, tag }: Row): string { return tag + id; }\n"+
			"function maybe(r?: Row): string { return r ? r.tag : 'none'; }\n"+
			"function withDefault(r: Row = { id: 0, tag: 'd' }): string { return r.tag + r.id; }\n"+
			"function repeat(r: Row, n: number): string { if (n === 0) return r.tag; return repeat(r, n - 1) + '.'; }\n"+
			"function describe(r: Row): string {\n"+
			"  const own = () => r.tag.toUpperCase();\n"+
			"  return own() + typeof r + (r.id === 1) + JSON.stringify({ ...r, k: true });\n"+
			"}\n"+
			"function swap(r: Row): string { r = { id: 9, tag: 'z' }; return r.tag + r.id; }\n"+
			"const first = Object.values(m)[0];\n"+
			"console.log(pick(first), pick(Object.values(m)[1]), pick({ id: 7, tag: 'q' }));\n"+
			"console.log(label(first), label({ id: 8, tag: 'w' }));\n"+
			"console.log(join(first, Object.values(m)[1]));\n"+
			"console.log(outer(first));\n"+
			"console.log(count(Object.values(m)));\n"+
			"console.log(spell(first));\n"+
			"console.log(maybe(first), maybe());\n"+
			"console.log(withDefault(first), withDefault());\n"+
			"console.log(repeat(first, 3));\n"+
			"console.log(describe(first));\n"+
			"console.log(swap(first), first.tag);\n")
	want := "1 2 7\n" +
		"x#1 w#8\n" +
		"xy\n" +
		"x!\n" +
		"3\n" +
		"x1\n" +
		"x none\n" +
		"x1 d0\n" +
		"x...\n" +
		"Xobjecttrue{\"id\":1,\"tag\":\"x\",\"k\":true}\n" +
		"z9 x\n"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

// TestAFunctionThatAnswersABox is the return half of the same rule. A helper that pulls
// a piece out of parsed JSON:
//
//	function head(): Row { return Object.values(m)[0] }
//	head().tag;
//
// declares a shape it cannot build, since what every return actually holds is a box. So
// its Go result is a value.Value and the call is itself a box, which is what makes the
// read after it dispatch.
//
// Identity is the property this buys over a conversion: head() === head() is true here
// because both calls hand back the one object the parse built, which a struct copied out
// of the box at the return would have lost.
//
// Held against what Node v24.18.0 prints.
func TestAFunctionThatAnswersABox(t *testing.T) {
	got := buildAndRunFile(t, "main.ts",
		"type Row = { id: number; tag: string };\n"+
			"const m = JSON.parse('{\"a\":{\"id\":1,\"tag\":\"x\"},\"b\":{\"id\":2,\"tag\":\"y\"}}') as Record<string, Row>;\n"+
			"function head(): Row { return Object.values(m)[0]; }\n"+
			"function pickBy(key: string): Row { return m[key]; }\n"+
			"function relay(): Row { return head(); }\n"+
			"const h = head();\n"+
			"console.log(h.id, h.tag);\n"+
			"console.log(head().tag, head().id + 1);\n"+
			"console.log(head().tag.toUpperCase());\n"+
			"console.log(pickBy('b').tag, pickBy('b').id);\n"+
			"console.log(relay().tag);\n"+
			"console.log(JSON.stringify({ ...head(), k: 1 }));\n"+
			"console.log(JSON.stringify(head()));\n"+
			"console.log(typeof h, head() === head());\n"+
			"const rows = [head(), pickBy('b')];\n"+
			"console.log(rows.map((r) => r.tag).join(','));\n"+
			"console.log(head());\n")
	want := "1 x\n" +
		"x 2\n" +
		"X\n" +
		"y 2\n" +
		"x\n" +
		"{\"id\":1,\"tag\":\"x\",\"k\":1}\n" +
		"{\"id\":1,\"tag\":\"x\"}\n" +
		"object true\n" +
		"x,y\n" +
		"{ id: 1, tag: 'x' }\n"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

// TestAnArrowThatAnswersABox is the same rule at the other two ways to write a named
// function. A helper bound to a const:
//
//	const head = (): Row => Object.values(m)[0];
//
// answers a box exactly as the `function` form does, and it is the form the everyday
// helper takes. It reaches the answer from its own signature rather than through
// funcDeclNamed, so both the block body, which renders its result from the signature, and
// the concise body, which renders it from the body expression, have to arrive there.
//
// A concise arrow used to spell the declared struct as its result and return a
// value.Value into it, which is Go that does not build.
//
// Held against what Node v24.18.0 prints.
func TestAnArrowThatAnswersABox(t *testing.T) {
	got := buildAndRunFile(t, "main.ts",
		"type Row = { id: number; tag: string };\n"+
			"const m = JSON.parse('{\"a\":{\"id\":1,\"tag\":\"x\"},\"b\":{\"id\":2,\"tag\":\"y\"}}') as Record<string, Row>;\n"+
			"const head = (): Row => Object.values(m)[0];\n"+
			"const tail = (): Row => { return Object.values(m)[1]; };\n"+
			"const byKey = function (k: string): Row { return m[k]; };\n"+
			"const relay = (): Row => head();\n"+
			"const label = (r: Row): string => r.tag;\n"+
			"const h = head();\n"+
			"console.log(h.id, h.tag);\n"+
			"console.log(head().tag, head().id + 1);\n"+
			"console.log(tail().tag, byKey('a').id);\n"+
			"console.log(relay().tag, label(head()));\n"+
			"console.log(`t=${head().tag}`);\n"+
			"console.log(JSON.stringify({ ...head(), k: 1 }), head() === head());\n"+
			"const rows = [head(), byKey('b')];\n"+
			"console.log(rows.length, rows[1].tag);\n"+
			"for (const r of [head()]) { console.log(r.id, typeof r); }\n"+
			"console.log(head());\n")
	want := "1 x\n" +
		"x 2\n" +
		"y 1\n" +
		"x x\n" +
		"t=x\n" +
		"{\"id\":1,\"tag\":\"x\",\"k\":1} true\n" +
		"2 y\n" +
		"1 object\n" +
		"{ id: 1, tag: 'x' }\n"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

// TestReturnsThatDisagreeAnswerABox covers the body that hands back a box on one path and
// something it built itself on another, which is what a lookup with a fallback looks like:
//
//	function orLit(k: string, b: boolean): Row { if (b) return m[k]; return { id: 0, tag: 'd' } }
//
// One box settles it for the whole function, because a Go function has one result type and
// the box is the only one of the two the other returns can be brought to: the literal boxes
// on its way out through the ordinary return coercion, where a box has no way to become the
// struct. That is the parameter half's rule read at the result, where the static literal
// argument boxes into the slot the boxed call site decided.
//
// A returned ternary is the same disagreement written in one expression, and it lowers to a
// return in each arm, so the arms are what decide it.
//
// Held against what Node v24.18.0 prints.
func TestReturnsThatDisagreeAnswerABox(t *testing.T) {
	got := buildAndRunFile(t, "main.ts",
		"type Row = { id: number; tag: string };\n"+
			"const m = JSON.parse('{\"a\":{\"id\":1,\"tag\":\"x\"},\"b\":{\"id\":2,\"tag\":\"y\"}}') as Record<string, Row>;\n"+
			"function orLit(k: string, b: boolean): Row { if (b) return m[k]; return { id: 0, tag: 'd' }; }\n"+
			"function orTern(k: string, b: boolean): Row { return b ? m[k] : { id: 9, tag: 'z' }; }\n"+
			"function orLocal(b: boolean): Row { const s: Row = { id: 5, tag: 'v' }; if (b) return m['a']; return s; }\n"+
			"function orNested(b: boolean): Row { if (b) { if (b) return m['a']; } return { id: 0, tag: 'n' }; }\n"+
			"function orTry(k: string): Row { try { return m[k]; } catch { return { id: 0, tag: 'e' }; } }\n"+
			"function orSwitch(k: string, n: number): Row { switch (n) { case 1: return m[k]; default: return { id: 0, tag: 's' }; } }\n"+
			"function deep(k: string, n: number): Row { if (n <= 0) return m[k]; return deep(k, n - 1); }\n"+
			"console.log(orLit('a', true).tag, orLit('a', false).tag);\n"+
			"console.log(orTern('b', true).id, orTern('b', false).id);\n"+
			"console.log(orLocal(true).tag, orLocal(false).tag);\n"+
			"console.log(orNested(true).id, orNested(false).tag);\n"+
			"console.log(orTry('b').tag, orSwitch('a', 1).tag, orSwitch('a', 2).tag);\n"+
			"console.log(deep('b', 3).tag);\n"+
			"console.log(JSON.stringify(orLit('a', true)), JSON.stringify({ ...orLit('a', false) }));\n")
	want := "x d\n" +
		"2 9\n" +
		"x v\n" +
		"1 n\n" +
		"y x s\n" +
		"y\n" +
		"{\"id\":1,\"tag\":\"x\"} {\"id\":0,\"tag\":\"d\"}\n"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

// TestAMethodTakesAndAnswersABox covers the class half of the same two rules. A method is
// the everyday place this code puts a helper, and until now every box crossing one of its
// signatures handed back.
//
// A method's Go signature is written in more than one place as soon as there is a
// hierarchy, so the rewrite is only offered to a method of a class with no base and no
// subclass and no virtual dispatch, which is where the signature is written once and the
// same rewrite the top-level function takes applies unchanged: the parameter a call site
// boxes takes the value slot, the literal argument boxes on its way in, one returned box
// settles the result, and the read off the call dispatches.
//
// this.head() inside a sibling method is the shape that has to know it is holding a box,
// so the receiver of a call resolves through the class being lowered too.
//
// Held against what Node v24.18.0 prints.
func TestAMethodTakesAndAnswersABox(t *testing.T) {
	got := buildAndRunFile(t, "main.ts",
		"type Row = { id: number; tag: string };\n"+
			"const m = JSON.parse('{\"a\":{\"id\":1,\"tag\":\"x\"},\"b\":{\"id\":2,\"tag\":\"y\"}}') as Record<string, Row>;\n"+
			"class Store {\n"+
			"  take(r: Row): number { return r.id; }\n"+
			"  head(): Row { return Object.values(m)[0]; }\n"+
			"  byKey(k: string): Row { return m[k]; }\n"+
			"  label(r: Row): string { return r.tag; }\n"+
			"  join(a: Row, b: Row): string { return a.tag + b.tag; }\n"+
			"  spell({ id, tag }: Row): string { return tag + id; }\n"+
			"  copy(r: Row): string { return JSON.stringify({ ...r, k: 1 }); }\n"+
			"  pick(k: string, b: boolean): Row { if (b) return m[k]; return { id: 0, tag: 'd' }; }\n"+
			"  orLit(k: string, b: boolean): Row { return b ? m[k] : { id: 9, tag: 'z' }; }\n"+
			"  headTag(): string { return this.head().tag; }\n"+
			"  plus(n: number): number { return this.take(this.head()) + n; }\n"+
			"}\n"+
			"const s = new Store();\n"+
			"const first = Object.values(m)[0];\n"+
			"const v = Object.values(m);\n"+
			"console.log(s.take(first), s.take(m['b']), s.take({ id: 7, tag: 'q' }));\n"+
			"console.log(s.head().tag, s.head().id, s.byKey('b').tag);\n"+
			"console.log(s.label(s.head()), s.join(v[0], v[1]), s.spell(first));\n"+
			"console.log(s.copy(first));\n"+
			"console.log(s.pick('a', true).tag, s.pick('a', false).tag, s.orLit('b', true).tag, s.orLit('b', false).tag);\n"+
			"console.log(s.headTag(), s.plus(4));\n"+
			"console.log(`${s.head().tag}!`, s.head().id + 1);\n"+
			"console.log(JSON.stringify(s.head()), s.head() === s.head());\n"+
			"for (const k of Object.keys(m)) { console.log(k, s.byKey(k).tag); }\n"+
			"console.log(s.head());\n")
	want := "1 2 7\n" +
		"x 1 y\n" +
		"x xy x1\n" +
		"{\"id\":1,\"tag\":\"x\",\"k\":1}\n" +
		"x d y z\n" +
		"x 5\n" +
		"x! 2\n" +
		"{\"id\":1,\"tag\":\"x\"} true\n" +
		"a x\n" +
		"b y\n" +
		"{ id: 1, tag: 'x' }\n"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

// TestAStaticMethodTakesAndAnswersABox covers the static half. A static method is a
// package function the class name routes to, so its Go signature is written in one place
// no matter what the class hierarchy looks like, and it takes the rewrite on the plainest
// terms of any of the three shapes this rule now covers.
//
// Held against what Node v24.18.0 prints.
func TestAStaticMethodTakesAndAnswersABox(t *testing.T) {
	got := buildAndRunFile(t, "main.ts",
		"type Row = { id: number; tag: string };\n"+
			"const m = JSON.parse('{\"a\":{\"id\":1,\"tag\":\"x\"},\"b\":{\"id\":2,\"tag\":\"y\"}}') as Record<string, Row>;\n"+
			"class Reg {\n"+
			"  static take(r: Row): number { return r.id; }\n"+
			"  static head(): Row { return Object.values(m)[0]; }\n"+
			"  static byKey(k: string): Row { return m[k]; }\n"+
			"  static pick(b: boolean): Row { if (b) return m['a']; return { id: 0, tag: 'd' }; }\n"+
			"  static both(a: Row, b: Row): string { return a.tag + b.tag; }\n"+
			"}\n"+
			"console.log(Reg.take(Object.values(m)[0]), Reg.take({ id: 3, tag: 'q' }));\n"+
			"console.log(Reg.head().tag, Reg.byKey('b').id);\n"+
			"console.log(Reg.take(Reg.head()), Reg.pick(true).tag, Reg.pick(false).tag);\n"+
			"console.log(Reg.both(Reg.head(), Reg.byKey('b')));\n"+
			"console.log(JSON.stringify(Reg.head()), Reg.head() === Reg.head());\n")
	want := "1 3\n" +
		"x 2\n" +
		"1 x d\n" +
		"xy\n" +
		"{\"id\":1,\"tag\":\"x\"} true\n"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

// TestAGetterAnswersABox covers the accessor. A getter emits through the method path and
// takes no parameters, so only note 386's return half can apply, and it applies on the
// same terms: a body that hands back a box gives the getter a value.Value result.
//
// What is different is where the box arrives. A getter is called by being read, so the
// property read is the site that has to know it is holding one, and it is also why the
// read-as-a-value check that protects a method from rows.map(s.take) is not asked here:
// it would match every getter's own use. An unrelated object's property of the same name
// is a separate read that resolves to itself, so it neither takes the rewrite nor blocks
// it.
//
// A static getter is the package function half, read off the class name.
//
// Held against what Node v24.18.0 prints.
func TestAGetterAnswersABox(t *testing.T) {
	got := buildAndRunFile(t, "main.ts",
		"type Row = { id: number; tag: string };\n"+
			"const m = JSON.parse('{\"a\":{\"id\":1,\"tag\":\"x\"},\"b\":{\"id\":2,\"tag\":\"y\"}}') as Record<string, Row>;\n"+
			"class Store {\n"+
			"  on = true;\n"+
			"  get head(): Row { return Object.values(m)[0]; }\n"+
			"  get second(): Row { return m['b']; }\n"+
			"  get either(): Row { if (this.on) return m['a']; return { id: 0, tag: 'd' }; }\n"+
			"  get count(): number { return 5; }\n"+
			"  label(r: Row): string { return r.tag; }\n"+
			"  headTag(): string { return this.head.tag; }\n"+
			"}\n"+
			"class Reg { static get head(): Row { return m['a']; } }\n"+
			"const s = new Store();\n"+
			"const o = { head: 5 };\n"+
			"console.log(s.head.tag, s.head.id, s.second.tag);\n"+
			"console.log(s.label(s.head), s.headTag(), s.count + 1, o.head);\n"+
			"console.log(s.either.tag);\n"+
			"s.on = false;\n"+
			"console.log(s.either.tag);\n"+
			"console.log(`${s.head.tag}!`, s.head.id + 1);\n"+
			"console.log(JSON.stringify({ ...s.head, k: 2 }), s.head === s.head);\n"+
			"console.log(Reg.head.tag, JSON.stringify(Reg.head));\n"+
			"for (const k of Object.keys(m)) { console.log(k, s.second.tag); }\n"+
			"console.log(s.head);\n")
	want := "x 1 y\n" +
		"x x 6 5\n" +
		"x\n" +
		"d\n" +
		"x! 2\n" +
		"{\"id\":1,\"tag\":\"x\",\"k\":2} true\n" +
		"x {\"id\":1,\"tag\":\"x\"}\n" +
		"a y\n" +
		"b y\n" +
		"{ id: 1, tag: 'x' }\n"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

// TestAFieldHoldsABox covers the slot. A field is where a box comes to rest, and it takes
// the same rewrite the parameter and the result take, for the same reason: a Go struct
// field has one type, and of the two candidates the box is the only one the other stores
// can be brought to, since a static value boxes on its way in where a box has no way to
// become the struct.
//
// A store is a store wherever it is written, so the initializer, a this.f = v inside the
// class, and a recv.f = v from outside all decide it together. The reads that follow are
// the ones a box already answers: a member, a call argument, a template, a spread, JSON,
// console, and identity, which is the property that made this a rewrite rather than a
// conversion in the first place.
//
// The constructor is the half that faces the call site. Its parameter takes the value slot
// on the same terms, so new S(box) has somewhere to put what it is handed, and new S({...})
// boxes on its way in to agree with it.
//
// A field declared Row | undefined is boxed like any other and stops being a value.Opt
// slot, so the coercion asks for the type the field now has rather than the one it was
// written with.
//
// Held against what Node prints.
func TestAFieldHoldsABox(t *testing.T) {
	got := buildAndRunFile(t, "main.ts",
		"type Row = { id: number; tag: string };\n"+
			"const m = JSON.parse('{\"a\":{\"id\":1,\"tag\":\"x\"},\"b\":{\"id\":2,\"tag\":\"y\"}}') as Record<string, Row>;\n"+
			"function label(r: Row): string { return r.tag + r.id; }\n"+
			"class Store {\n"+
			"  first: Row = Object.values(m)[0];\n"+
			"  last: Row = { id: 0, tag: 'z' };\n"+
			"  spare: Row | undefined = m['b'];\n"+
			"  n = 3;\n"+
			"  head(): Row { return m['a']; }\n"+
			"  keep(): void { this.last = this.head(); }\n"+
			"  pick(k: string): void { this.first = m[k]; }\n"+
			"  get(): Row { return this.first; }\n"+
			"  tag(): string { return this.first.tag; }\n"+
			"}\n"+
			"class Holder {\n"+
			"  r: Row;\n"+
			"  n: number;\n"+
			"  constructor(r: Row) { const t = r.tag; this.r = r; this.n = t.length; }\n"+
			"}\n"+
			"const s = new Store();\n"+
			"console.log(s.first.tag, s.first.id, s.n, s.spare?.tag);\n"+
			"console.log(s.last.tag);\n"+
			"s.keep();\n"+
			"console.log(s.last.tag, s.get().tag, s.tag());\n"+
			"s.pick('b');\n"+
			"console.log(s.first.tag, label(s.first), `${s.first.tag}!`);\n"+
			"console.log(JSON.stringify(s.first), s.first === s.first);\n"+
			"console.log(JSON.stringify({ ...s.first, k: 1 }));\n"+
			"s.last = m['a'];\n"+
			"console.log(s.last.id, s.last);\n"+
			"console.log(new Holder(Object.values(m)[0]).r.tag, new Holder(m['b']).n);\n"+
			"console.log(new Holder({ id: 7, tag: 'q' }).r.tag);\n")
	want := "x 1 3 y\n" +
		"z\n" +
		"x x x\n" +
		"y y2 y!\n" +
		"{\"id\":2,\"tag\":\"y\"} true\n" +
		"{\"id\":2,\"tag\":\"y\",\"k\":1}\n" +
		"1 { id: 1, tag: 'x' }\n" +
		"x 1\n" +
		"q\n"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

// TestAFieldHoldsABoxInAHierarchy lifts note 389's boundary for the field. A method had to
// stop at a hierarchy because its Go signature is written again at the vtable and at the
// interface every derived receiver is called through. A field is not: it lives in the
// struct of the class that declares it and nowhere else, a derived struct embeds its base
// and reaches it through Go's own promotion, and registration rejects a derived member
// sharing a base member's name, so at most one class on a chain owns any property. The
// one-place condition the whole pass rests on holds however deep the chain is.
//
// So a base's field, a derived class's own field, a store written in a derived class, a
// read from a base method every subclass inherits, and a read through a base-typed
// binding all reach the same value slot. A virtual method still dispatches; what changed
// is only what the field it reads holds.
//
// The constructor comes along for the same reason, being a package function written once
// per class. super(r) is where the box reaches the base's parameter, since a derived class
// hands its own parameter straight on and only the base's declaration says what the field
// it fills holds.
//
// Held against what Node prints.
func TestAFieldHoldsABoxInAHierarchy(t *testing.T) {
	got := buildAndRunFile(t, "main.ts",
		"type Row = { id: number; tag: string };\n"+
			"const m = JSON.parse('{\"a\":{\"id\":1,\"tag\":\"x\"},\"b\":{\"id\":2,\"tag\":\"y\"}}') as Record<string, Row>;\n"+
			"abstract class Base {\n"+
			"  first: Row = Object.values(m)[0];\n"+
			"  cur: Row = { id: 0, tag: 'z' };\n"+
			"  abstract kind(): string;\n"+
			"  show(): string { return this.kind() + this.first.tag; }\n"+
			"  label(): string { return 'B' + this.first.id; }\n"+
			"}\n"+
			"class Mid extends Base {\n"+
			"  own: Row = m['b'];\n"+
			"  kind(): string { return 'm'; }\n"+
			"  load(k: string): void { this.cur = m[k]; }\n"+
			"}\n"+
			"class Leaf extends Mid {\n"+
			"  n = 7;\n"+
			"  label(): string { return 'L' + this.first.tag + this.own.id; }\n"+
			"}\n"+
			"class Held { r: Row; constructor(r: Row) { this.r = r; } }\n"+
			"class HeldMore extends Held {\n"+
			"  n: number;\n"+
			"  constructor(r: Row, n: number) { super(r); this.n = n; }\n"+
			"}\n"+
			"const leaf = new Leaf();\n"+
			"console.log(leaf.first.tag, leaf.own.tag, leaf.cur.tag, leaf.n);\n"+
			"leaf.load('a');\n"+
			"console.log(leaf.cur.tag, leaf.show(), JSON.stringify(leaf.first));\n"+
			"const xs: Base[] = [new Mid(), leaf];\n"+
			"for (const x of xs) { console.log(x.label(), x.show()); }\n"+
			"const b: Mid = leaf;\n"+
			"console.log(b.first.id, b.own.tag, b.first === leaf.first);\n"+
			"console.log(new HeldMore(m['b'], 4).r.tag, new HeldMore({ id: 5, tag: 'p' }, 1).r.id);\n"+
			"console.log(new Held(Object.values(m)[0]).r.tag);\n")
	want := "x y z 7\n" +
		"x mx {\"id\":1,\"tag\":\"x\"}\n" +
		"B1 mx\n" +
		"Lx2 mx\n" +
		"1 y true\n" +
		"y 5\n" +
		"x\n"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

// TestAnInlineCallbacksParameterTakesABox holds the whole slice against Node. A callback
// written inline into a call on a box is handed its arguments boxed, so its parameter
// holds a box and everything the body does with that parameter is a read through the
// value model: passing it on to a declared function, to a method, to a constructor, into
// a nested callback, spreading it, comparing it by identity, storing it into a field.
func TestAnInlineCallbacksParameterTakesABox(t *testing.T) {
	got := buildAndRunFile(t, "main.ts",
		"type Row = { id: number; tag: string };\n"+
			"const m = JSON.parse('{\"a\":{\"id\":1,\"tag\":\"x\"},\"b\":{\"id\":2,\"tag\":\"y\"}}') as Record<string, Row>;\n"+
			"\n"+
			"function label(r: Row): string { return r.tag + r.id; }\n"+
			"function pick(r: Row): Row { return r; }\n"+
			"\n"+
			"class Holder {\n"+
			"  first: Row = { id: 0, tag: 'z' };\n"+
			"  seen: Row;\n"+
			"  constructor(r: Row) { this.seen = r; }\n"+
			"  name(r: Row): string { return 'h' + r.tag; }\n"+
			"  all(): string { return Object.values(m).map((r: Row) => this.name(r)).join(','); }\n"+
			"}\n"+
			"\n"+
			"console.log(Object.values(m).map((r: Row) => r.tag).join(','));\n"+
			"console.log(Object.values(m).map((r: Row) => label(r)).join(','));\n"+
			"console.log(Object.values(m).map((r: Row) => pick(r).tag).join(','));\n"+
			"console.log(Object.values(m).map(function (r: Row) { return label(r); }).join(','));\n"+
			"console.log(Object.values(m).map((r: Row) => { return r; }).map((r: Row) => r.id).join(','));\n"+
			"console.log(Object.values(m).map(({ tag, id }: Row) => tag + id).join(','));\n"+
			"console.log(Object.values(m).map((r: Row, i: number) => `${i}${r.tag}`).join(','));\n"+
			"console.log(Object.values(m).map((r: Row) => JSON.stringify({ ...r, k: 1 })).join('|'));\n"+
			"console.log(Object.values(m).map((r: Row) => r.id > 1 ? r : { id: 0, tag: 'z' }).map((r: Row) => r.tag).join(','));\n"+
			"console.log(Object.values(m).map((r: Row) => [r].map((q: Row) => label(q)).join('')).join(','));\n"+
			"\n"+
			"const first = Object.values(m)[0];\n"+
			"const same = Object.values(m).filter((r: Row) => r === first);\n"+
			"console.log(same.length, same[0] === first);\n"+
			"\n"+
			"const h = new Holder(Object.values(m)[1]);\n"+
			"Object.values(m).forEach((r: Row) => { h.first = r; });\n"+
			"console.log(h.first.tag, h.seen.tag, h.all());\n"+
			"console.log(Object.values(m).map((r: Row) => new Holder(r).seen.tag).join(','));\n"+
			"console.log(Object.values(m).reduce((acc: number, r: Row) => acc + r.id, 0));\n"+
			"console.log(Object.values(m).sort((a: Row, b: Row) => b.id - a.id).map((r: Row) => r.tag).join(','));\n")
	want := "x,y\n" +
		"x1,y2\n" +
		"x,y\n" +
		"x1,y2\n" +
		"1,2\n" +
		"x1,y2\n" +
		"0x,1y\n" +
		"{\"id\":1,\"tag\":\"x\",\"k\":1}|{\"id\":2,\"tag\":\"y\",\"k\":1}\n" +
		"z,y\n" +
		"x1,y2\n" +
		"1 true\n" +
		"y y hx,hy\n" +
		"x,y\n" +
		"3\n" +
		"y,x\n"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}
