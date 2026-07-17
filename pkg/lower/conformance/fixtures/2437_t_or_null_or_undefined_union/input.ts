function f(x: number | null | undefined): string {
  if (x === null) return "null";
  if (x === undefined) return "undef";
  return String(x + 1);
}
function truthy(x: string | null | undefined): string {
  return x ? "t" : "f";
}
console.log(f(5));
console.log(f(null));
console.log(f(undefined));
console.log(truthy("hi"));
console.log(truthy(""));
console.log(truthy(null));
console.log(truthy(undefined));
