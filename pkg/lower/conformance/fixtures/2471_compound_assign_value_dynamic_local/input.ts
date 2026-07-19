// A compound assignment used for its value, (x += n) in an expression position,
// yields the updated target the way JavaScript's assignment operator does. When
// the target local is stored as a box (a dynamic local here), the read-modify-write
// runs in a closure that returns the box; a dynamic-context use keeps the box, and
// a static-primitive context coerces it down through the ToNumber family.
let x: any = 10;
const a = (x += 5);
console.log(a);
const n: number = (x += 3);
console.log(n);
console.log(x);
