// A function whose return type is a general union of two unlike primitives lowers
// to the tagged sum of section 9: number | string becomes a value struct with a
// discriminant tag and one inline field per arm, and each return wraps its value in
// the arm constructor so the tag and the payload never drift. A caller binds the
// result to a local of the union type and passes it on, threading the sum through
// the program without ever guessing a single Go type for a value that is one of two.
function pick(b: boolean): number | string {
  return b ? 1 : "a";
}

function first(a: number | string, b: number | string): number | string {
  return a;
}

function show(v: number | string): string {
  if (typeof v === "string") {
    return v;
  }
  return String(v);
}

function run(): void {
  const x = pick(true);
  const y = pick(false);
  console.log(show(x));
  console.log(show(y));
  console.log(show(first(x, y)));
  console.log(show(first(pick(false), pick(true))));
}

run();
