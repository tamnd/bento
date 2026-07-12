// Reflect.defineProperty applies a descriptor and reports whether the define
// succeeded, returning false where Object.defineProperty would throw. A fresh
// define on an extensible object succeeds.
const o: any = {};
console.log(Reflect.defineProperty(o, "x", { value: 5, enumerable: true, configurable: true })); // true
console.log(o.x); // 5

// A non-configurable property refuses a redefine and reports false, leaving the
// original value in place.
Reflect.defineProperty(o, "y", { value: 1, configurable: false });
console.log(Reflect.defineProperty(o, "y", { value: 2 })); // false
console.log(o.y); // 1

// Reflect.getOwnPropertyDescriptor answers for a present key and reports undefined
// for an absent one. Its PropertyDescriptor | undefined result is a union bento does
// not yet destructure, so the fields are pinned by the runtime unit test and here
// only its presence is observed.
console.log(Reflect.getOwnPropertyDescriptor(o, "x") !== undefined); // true
console.log(Reflect.getOwnPropertyDescriptor(o, "missing") === undefined); // true

// Reflect.setPrototypeOf installs a prototype and reports success, so an inherited
// property resolves through the child. Reflect.getPrototypeOf reads the slot back;
// its object | null result is a union bento does not yet consume, so the call is
// exercised here and its value pinned by the runtime unit test.
const proto: any = { greet: "hi" };
const child: any = {};
console.log(Reflect.setPrototypeOf(child, proto)); // true
console.log(child.greet); // hi
Reflect.getPrototypeOf(child);

// Changing the prototype of a non-extensible object to a different one is refused
// and reports false.
const frozen: any = {};
Object.preventExtensions(frozen);
console.log(Reflect.setPrototypeOf(frozen, proto)); // false
