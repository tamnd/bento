// A union of object shapes that all declare the same keys has no discriminant and
// needs none. The checker answers a read of any key with the union of what the
// members hold there, so the union is one struct whose fields are those unions.

type U = { a: number, b: string } | { a: string, b: number };

function show(name: string, v: unknown): void {
  console.log(name + " => " + JSON.stringify(v));
}

// The shape node's own test harness opens with: a ternary picking between two
// literals of the same keys, whose branches disagree only in the types they hold.
const ms: number | bigint = 300;
const multipliers = typeof ms === "bigint"
  ? { two: 2n, four: 4n, seven: 7n }
  : { two: 2, four: 4, seven: 7 };
show("tern-two", typeof multipliers.two);
show("tern-four", typeof multipliers.four);
show("tern-seven", typeof multipliers.seven);
show("tern-json", JSON.stringify(multipliers));

// A binding annotated at the union, read back at both keys.
const bound: U = { a: 1, b: "y" };
show("bind-a", bound.a);
show("bind-b", bound.b);
show("bind-typeof-a", typeof bound.a);
show("bind-json", JSON.stringify(bound));
show("bind-keys", Object.keys(bound).join(","));
show("bind-in", "a" in bound);
show("bind-missing-in", "c" in bound);

// A return at the union, from both an if and a ternary, and an argument at it.
function fromIf(x: boolean): U {
  if (x) { return { a: 1, b: "y" }; }
  return { a: "z", b: 2 };
}
function fromTernary(x: boolean): U {
  return x ? { a: 1, b: "y" } : { a: "z", b: 2 };
}
function nameOfA(u: U): string { return typeof u.a; }
show("ret-if-true", nameOfA(fromIf(true)));
show("ret-if-false", nameOfA(fromIf(false)));
show("ret-tern-true", nameOfA(fromTernary(true)));
show("ret-tern-false", nameOfA(fromTernary(false)));
show("arg-literal", nameOfA({ a: 1, b: "y" }));
show("arg-literal-other", nameOfA({ a: "z", b: 2 }));

// A value that flows through the union unchanged keeps both its keys.
function passThrough(u: U): U { return u; }
show("pass-a", nameOfA(passThrough({ a: "z", b: 2 })));

// An assignment writes the other member into the same slot.
let slot: U = { a: 1, b: "y" };
show("slot-before", typeof slot.a);
slot = { a: "z", b: 2 };
show("slot-after", typeof slot.a);
show("slot-json", JSON.stringify(slot));

// An array whose element type is the union holds both members side by side.
const xs: U[] = [{ a: 1, b: "y" }, { a: "z", b: 2 }];
show("arr-len", xs.length);
show("arr-0", typeof xs[0].a);
show("arr-1", typeof xs[1].a);
show("arr-json", JSON.stringify(xs));

// A field declared at the union inside a wider shape builds at the merge too.
type W = { u: U, n: number };
const w: W = { u: { a: 1, b: "y" }, n: 3 };
show("nested-a", typeof w.u.a);
show("nested-n", w.n);
show("nested-json", JSON.stringify(w));

// Narrowing the receiver narrows every read off it, and the read still answers the
// value the slot holds.
function describe(u: U): string {
  if (typeof u.a === "number") { return "num:" + (u.a + 1); }
  return "str:" + u.a.length;
}
show("narrow-num", describe(fromIf(true)));
show("narrow-str", describe(fromIf(false)));

// Members that agree at a key keep that key's own type, so a write to it is an
// ordinary field write.
type V = { a: number, b: string } | { a: number, b: number };
const v: V = { a: 1, b: "y" };
show("same-a", v.a);
show("same-typeof-b", typeof v.b);
