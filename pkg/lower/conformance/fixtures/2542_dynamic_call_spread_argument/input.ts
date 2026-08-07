function two(a: any, b: any): void {
  console.log("two", String(a), String(b));
}

function three(a: any, b: any, c: any): void {
  console.log("three", String(a), String(b), String(c));
}

const f2: any = two;
const f3: any = three;

// A tuple spread: static arity, but it is drained like every other operand so the
// receiver is evaluated once.
const tup: [string, number] = ["x", 2];
f2(...tup);

// An array whose element type is a plain primitive.
const arr: string[] = ["p", "q"];
f2(...arr);

// An operand that was already a box, which the drain walks without a static shape.
const boxed: any = ["m", "n"];
f2(...boxed);

// A fixed argument on either side of a spread lands where the source put it.
f3("lead", ...tup);
f3(...tup, "trail");

// A string spreads one element per code point, and a Set spreads its members in
// insertion order with the duplicate already dropped.
const s: string = "hi";
f2(...s);
const st = new Set<number>([1, 2, 2]);
f2(...st);

// The operand runs exactly once, which is why it is boxed and drained rather than
// read position by position.
let calls = 0;
function mk(): string[] {
  calls++;
  return ["u", "v"];
}
f2(...mk());
console.log("calls", calls);

// A spliced argument list still binds the receiver, so o.m(...xs) gets its `this`.
const o: any = {
  m(a: any, b: any) {
    console.log("method", String(a), String(b));
  },
};
o.m(...tup);

// An empty operand contributes nothing at all.
const empty: string[] = [];
f2(...empty);

// Spreading something that is not iterable throws the TypeError the language throws,
// rather than quietly passing no arguments.
const plain: any = { x: 1 };
try {
  f2(...plain);
  console.log("unreachable");
} catch (e) {
  console.log("threw", (e as Error).message);
}
