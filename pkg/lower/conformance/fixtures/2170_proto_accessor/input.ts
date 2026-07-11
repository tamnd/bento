const base: any = { greet: "hi" };
const obj: any = { __proto__: base, own: 1 };
console.log(obj.greet);
console.log(obj.own);
console.log(obj.__proto__ === base);

const other: any = { tag: "T" };
obj.__proto__ = other;
console.log(obj.tag);
console.log(obj.greet);
console.log(obj.__proto__ === other);

obj.__proto__ = 5;
console.log(obj.__proto__ === other);
console.log(obj.tag);

const bare: any = { __proto__: null };
console.log(Object.getPrototypeOf(bare) === null);
