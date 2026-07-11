const a: any = { x: 1 };
console.log(Object.isExtensible(a));
Object.preventExtensions(a);
console.log(Object.isExtensible(a));

const b: any = { y: 1 };
Object.seal(b);
console.log(Object.isExtensible(b));

const c: any = { z: 1 };
Object.freeze(c);
console.log(Object.isExtensible(c));
