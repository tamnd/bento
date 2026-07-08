const a = ['x', 0, -0];
console.log(String(a.length));
const b: (string | number)[] = ['a', 1, 'b', 2];
console.log(String(b.length));
console.log(JSON.stringify(['-0', 0, -0]));
console.log(JSON.stringify([1, 'a', true]));
