function f(a: number | null): number {
  return a ?? 5;
}
function g(c: string | null | undefined): string {
  return c ?? "x";
}
let hits = 0;
function bump(): number {
  hits++;
  return 1;
}
function h(v: number | null): number {
  return v ?? bump();
}
console.log(f(null));
console.log(f(7));
console.log(g(null));
console.log(g(undefined));
console.log(g("keep"));
console.log(h(3), hits);
console.log(h(null), hits);
