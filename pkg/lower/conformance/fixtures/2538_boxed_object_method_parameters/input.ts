// An object literal that flows into a dynamic slot boxes into the runtime's property
// bag, and each of its methods becomes a callable the object protocol invokes. A method
// that names parameters lowers to a wrapper that reads each argument off the boxed
// argument list, coerces it into the parameter's slot, runs the body, and boxes the
// result back, so calling it through the box behaves as the direct call would.
//
// take's parameter is typed any, which is what makes the literal box. Everything below
// is read back off the boxed object, so every call goes through the runtime's own
// dispatch rather than a Go method call the checker could see.

function take(x: any): any {
  return x;
}

const o: any = take({
  // Two parameters, the plainest form: both arguments land in their slots.
  add(a: number, b: number): number {
    return a + b;
  },
  // A string parameter, so the wrapper coerces the box down to a string rather than a
  // number.
  greet(name: string): string {
    return "hi " + name;
  },
  // No parameters, the shape the coercion methods take. It keeps the compact closure
  // that ignores the argument list, so nothing that lowered before pays for a wrapper.
  zero(): number {
    return 0;
  },
  // A block body with a local and a loop, lowered the same way an arrow's block body is.
  sum(n: number): number {
    let t = 0;
    for (let i = 0; i < n; i++) {
      t += i;
    }
    return t;
  },
  // Two returns of different string literals, so the return type is a closed literal
  // union that boxes as the string it observably is.
  size(n: number): "big" | "small" {
    if (n > 2) {
      return "big";
    }
    return "small";
  },
  // The same with numbers, through a conditional rather than two statements.
  sign(n: number): number {
    return n < 0 ? -1 : 1;
  },
  // A defaulted parameter: the callee applies its own default, so a short call still
  // fills the slot.
  step(a: number, b: number = 10): number {
    return a + b;
  },
  // An object crossing the boundary in both directions. The parameter is typed any, so
  // the wrapper hands the box straight through and the body reads it back through the
  // runtime; the result is a fresh literal, which boxes on the way out. A parameter
  // declared at a fixed shape is a later slice: landing a box in a Go struct slot needs
  // a coercion the wrapper does not have yet.
  pair(p: any): { y: number } {
    return { y: p.x * 2 };
  },
  // A body that throws, so the error crosses back out of the boxed call.
  check(n: number): number {
    if (n < 0) {
      throw new Error("negative: " + n);
    }
    return n;
  },
  // A method whose result is fed straight back into another call on the same box.
  twice(n: number): number {
    return n * 2;
  },
  // A void method, which runs for its effect and answers undefined.
  say(m: string): void {
    console.log("say", m);
  },
});

console.log("add", o.add(2, 3));
console.log("greet", o.greet("bob"));
console.log("zero", o.zero());
console.log("sum", o.sum(4));
console.log("size-big", o.size(5));
console.log("size-small", o.size(1));
console.log("sign", o.sign(-3), o.sign(3));
console.log("step-two", o.step(1, 2));
console.log("step-one", o.step(1));
console.log("pair", o.pair({ x: 4 }).y);
console.log("twice-chain", o.twice(o.twice(3)));
console.log("say-result", o.say("hello"));

// A call with fewer arguments than the method names leaves the missing slot undefined,
// which the number coercion reads as NaN, exactly what the direct call would compute.
console.log("short", o.add(1));

try {
  o.check(-1);
} catch (e) {
  console.log("threw", (e as Error).message);
}
console.log("check", o.check(7));

// A well-known computed name is a method too. [Symbol.toPrimitive] takes the hint the
// language passes when it coerces the object, so the parameter arrives from the runtime
// rather than from source. It answers a string for every hint here: a method whose
// return type is a union mixing two primitives is a later slice, since the union has no
// single Go type the box could read.
const c: any = take({
  [Symbol.toPrimitive](hint: string): string {
    return "as-" + hint;
  },
});
console.log("hint-string", `${c}`);
console.log("hint-default", c + "");
