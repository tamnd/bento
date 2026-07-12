interface Inner { x: number; y?: number; }
interface Rec { a: number; b?: number; c?: string; inner?: Inner; }
const present: Rec = { a: 1, b: 2, c: "x", inner: { x: 10 } };
console.log(JSON.stringify(present));
const absent: Rec = { a: 1 };
console.log(JSON.stringify(absent));
const undef: Rec = { a: 1, b: undefined, inner: { x: 5, y: 6 } };
console.log(JSON.stringify(undef));
