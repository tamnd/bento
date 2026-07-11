const o: { p: { x: number; y: number }; q: { z: number } } = { p: { x: 1, y: 2 }, q: { z: 3 } };
let x = 0, y = 0, z = 0;
({ p: { x, y }, q: { z } } = o);
console.log(x);
console.log(y);
console.log(z);
