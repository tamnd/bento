// An object that carries a Symbol.toStringTag string overrides the default tag
// Object.prototype.toString reports, so the borrowed toString names the instance by
// that tag rather than "[object Object]"; a plain object with no such property keeps
// the default tag.
const o: any = {};
o[Symbol.toStringTag] = "Widget";
console.log(Object.prototype.toString.call(o));
const plain: any = {};
console.log(Object.prototype.toString.call(plain));
