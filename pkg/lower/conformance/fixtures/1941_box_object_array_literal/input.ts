// An object or array literal bound into a dynamic slot stores a live boxed value,
// not the static struct or slice a fixed-shape binding would hold, so the members
// read back through dynamic access the way a JavaScript object does. The literal
// builds value.NewObject().Set(...) and value.NewArrayValue(...) straight from its
// members rather than interning a Go struct the any slot never names. The test262
// harness leans on this everywhere it stashes a descriptor or an options bag in an
// untyped variable, so boxing the literal is the gate those includes wait behind.
const a: any = [10, 20, 30];
console.log(String(a[0]));
console.log(String(a.length));

const o: any = { x: 1, y: "hi" };
console.log(String(o.x));
console.log(String(o.y));

const nested: any = { items: [1, 2], label: "pair" };
console.log(String(nested.items[1]));
console.log(String(nested.label));
