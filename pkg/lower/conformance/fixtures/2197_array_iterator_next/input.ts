// The array iterator's next() yields { value, done } for each kind. The values
// kind hands back each element and then a done result whose value is undefined,
// the keys kind hands back each index, and the entries kind reports done once its
// pairs run out.
const a = [10, 20];

const vi = a.values();
let r = vi.next();
console.log(r.done);
console.log(r.value);
r = vi.next();
console.log(r.value);
r = vi.next();
console.log(r.done);
console.log(r.value === undefined);

const ki = a.keys();
console.log(ki.next().value);
console.log(ki.next().value);

const ei = a.entries();
console.log(ei.next().done);
console.log(ei.next().done);
console.log(ei.next().done);
