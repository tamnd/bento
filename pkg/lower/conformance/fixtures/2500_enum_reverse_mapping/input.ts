enum Color { R, G, B }
console.log(Color[0], Color[1], Color[2]);
const n: number = 2;
console.log(Color[n]);
console.log(Color[Color.G]);
enum E { A = 1, B = 5, C = 10 }
const m: number = 5;
console.log(E[m], E[10], E[1]);
enum D { A = 0, B = 0 }
console.log(D[0]);
