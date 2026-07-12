// Reflect.isExtensible reports whether an object still accepts new own properties,
// and Reflect.preventExtensions closes it, reporting success. A fresh object is
// extensible; once prevented it is not, while its existing properties stay readable.
const o: any = { a: 1 };
console.log(Reflect.isExtensible(o)); // true
console.log(Reflect.preventExtensions(o)); // true
console.log(Reflect.isExtensible(o)); // false
console.log(o.a); // 1
