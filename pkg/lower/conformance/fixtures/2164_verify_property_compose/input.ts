function verifyProperty(obj: any, name: string, desc: any): boolean {
  const d: any = Object.getOwnPropertyDescriptor(obj, name);
  if (d === undefined) return false;
  if (d.value !== desc.value) return false;
  if (d.writable !== desc.writable) return false;
  if (d.enumerable !== desc.enumerable) return false;
  if (d.configurable !== desc.configurable) return false;
  if (desc.configurable) {
    delete obj[name];
    const gone: any = Object.getOwnPropertyDescriptor(obj, name);
    if (gone !== undefined) return false;
    Object.defineProperty(obj, name, desc);
  }
  return true;
}

const o: any = {};
Object.defineProperty(o, "a", { value: 1, writable: true, enumerable: true, configurable: true });
console.log(verifyProperty(o, "a", { value: 1, writable: true, enumerable: true, configurable: true }));

Object.defineProperty(o, "hidden", { value: 2, writable: false, enumerable: false, configurable: true });
console.log(verifyProperty(o, "hidden", { value: 2, writable: false, enumerable: false, configurable: true }));

console.log(verifyProperty(o, "a", { value: 999, writable: true, enumerable: true, configurable: true }));
console.log(o.a);
