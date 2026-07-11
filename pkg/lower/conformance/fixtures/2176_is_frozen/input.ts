const a: any = { x: 1 };
console.log(Object.isFrozen(a));
Object.seal(a);
console.log(Object.isFrozen(a));

const b: any = { y: 1 };
Object.freeze(b);
console.log(Object.isFrozen(b));

const empty: any = {};
Object.seal(empty);
console.log(Object.isFrozen(empty));

const arr: any = [1];
Object.freeze(arr);
console.log(Object.isFrozen(arr));
