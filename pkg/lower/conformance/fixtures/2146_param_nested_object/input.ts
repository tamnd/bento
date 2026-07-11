function f({ p: { x, y } }: { p: { x: number; y: number } }): number {
  return x + y;
}
console.log(f({ p: { x: 1, y: 2 } }));
