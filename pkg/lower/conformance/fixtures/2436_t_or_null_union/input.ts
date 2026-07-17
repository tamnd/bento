function num(x: number | null): string {
  if (x === null) return "null";
  return String(x + 1);
}
function str(x: string | null): number {
  if (x === null) return -1;
  return x.length;
}
function bool(x: boolean | null): string {
  if (x === null) return "n";
  return x ? "yes" : "no";
}
function truthy(x: number | null): string {
  return x ? "t" : "f";
}
console.log(num(5));
console.log(num(null));
console.log(str("hello"));
console.log(str(null));
console.log(bool(true));
console.log(bool(null));
console.log(truthy(0));
console.log(truthy(4));
console.log(truthy(null));
