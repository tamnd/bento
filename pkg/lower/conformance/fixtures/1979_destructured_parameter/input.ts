// A destructured parameter binds names out of the argument rather than naming the
// argument itself. Go has no destructuring parameter, so the whole object or array
// arrives in one synthesized field and the bound names are read from it at the top
// of the body: an object pattern reads each field through its selector, an array
// pattern reads each position by index. A plain parameter beside a pattern is
// unaffected, so a mixed list binds the plain name and the pattern names together.

function area({ w, h }: { w: number; h: number }): number {
  return w * h;
}

function diff([x, y]: number[]): number {
  return x - y;
}

function label({ name, id }: { name: string; id: number }): string {
  return name + ":" + id;
}

function shift(base: number, { by }: { by: number }): number {
  return base + by;
}

console.log(area({ w: 3, h: 4 }));
console.log(diff([9, 4]));
console.log(label({ name: "n", id: 7 }));
console.log(shift(10, { by: 5 }));
