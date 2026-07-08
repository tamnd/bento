// Nullish coalescing short-circuits: the fallback runs only when the left is
// nullish. A pure fallback can run eagerly without being observed, but a fallback
// that calls a function or otherwise has a side effect must not fire when the left
// is present. Both the optional T | undefined left and the dynamic left keep that
// short-circuit, so the loud fallback below prints only on the nullish cases.

function loud(tag: string, v: number): number {
  console.log("fb:" + tag);
  return v;
}

function opt(x: number | undefined, tag: string): number {
  return x ?? loud(tag, -1);
}

function dyn(x: any, tag: string): any {
  return x ?? loud(tag, -2);
}

console.log(opt(5, "a"));
console.log(opt(undefined, "b"));
console.log(dyn(0, "c"));
console.log(dyn(undefined, "d"));
