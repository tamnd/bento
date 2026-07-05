// A rest parameter gathers the trailing arguments into an array. Go has no rest
// parameter, so it lowers to a final field of the parameter's array type and every
// call packs its extra arguments into that array at the call site, an empty call
// gathering an empty array. A fixed parameter before the rest keeps its position,
// and a defaulted fixed parameter fills its slot before the rest gathers the
// remainder, so the two call-site rewrites compose.
function sum(...ns: number[]): number {
  let t = 0;
  for (const n of ns) t = t + n;
  return t;
}

function join(sep: string, ...parts: string[]): string {
  let s = "";
  for (let i = 0; i < parts.length; i++) {
    if (i > 0) s = s + sep;
    s = s + parts[i];
  }
  return s;
}

function tally(base: number = 100, ...rest: number[]): number {
  let t = base;
  for (const r of rest) t = t + r;
  return t;
}

console.log(sum(1, 2, 3, 4));
console.log(sum());
console.log(join("-", "a", "b", "c"));
console.log(join("+", "x"));
console.log(tally());
console.log(tally(1, 2, 3));
