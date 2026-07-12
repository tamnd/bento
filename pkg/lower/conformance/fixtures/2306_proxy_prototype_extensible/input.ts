// The getPrototypeOf and setPrototypeOf traps route the prototype protocol, and the
// isExtensible and preventExtensions traps route the extensibility protocol. Each
// trap keeps its target invariant: isExtensible must agree with the target, and a
// truthy preventExtensions is honored only once the target is itself sealed.
const proto: any = { kind: "base" };
const target: any = {};
const p: any = new Proxy(target, {
  getPrototypeOf: (t: any): any => proto,
  isExtensible: (t: any): boolean => Reflect.isExtensible(t),
  preventExtensions: (t: any): boolean => {
    Object.preventExtensions(t);
    return true;
  },
});
console.log(Object.getPrototypeOf(p).kind); // base
console.log(Object.isExtensible(p)); // true
Object.preventExtensions(p);
console.log(Object.isExtensible(p)); // false

const p2: any = new Proxy({}, {
  setPrototypeOf: (t: any, pr: any): boolean => {
    Reflect.setPrototypeOf(t, pr);
    return true;
  },
});
Object.setPrototypeOf(p2, proto);
console.log(Object.getPrototypeOf(p2).kind); // base
