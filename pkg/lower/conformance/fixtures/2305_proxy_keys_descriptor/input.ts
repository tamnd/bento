// The ownKeys trap supplies the key list, the getOwnPropertyDescriptor trap
// supplies the descriptor each key reports, and the defineProperty trap intercepts
// a definition. Object.keys walks the proxy through ownKeys and then filters each
// key through getOwnPropertyDescriptor for the enumerable flag.
const log: string[] = [];
const target: any = {};
const keys: any = ["a", "b"];
const p: any = new Proxy(target, {
  ownKeys: (t: any): any => keys,
  getOwnPropertyDescriptor: (t: any, key: string): any => {
    const d: any = {
      value: key + "!",
      writable: true,
      enumerable: key === "a",
      configurable: true,
    };
    return d;
  },
  defineProperty: (t: any, key: string, desc: any): boolean => {
    log.push("define:" + key);
    return true;
  },
});
console.log(Object.getOwnPropertyNames(p).join(",")); // a,b
console.log(Object.keys(p).join(",")); // a
const d: any = Object.getOwnPropertyDescriptor(p, "a");
console.log(d.value); // a!
Object.defineProperty(p, "z", { value: 1 });
console.log(log.join(",")); // define:z
