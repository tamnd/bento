// A get trap intercepts every property read, receiving the target, the key, and
// the proxy as the receiver, and its return value is what the read yields. A set
// trap intercepts every write and decides whether the write reaches the target.
const log: string[] = [];
const target: any = { x: 1 };
const p: any = new Proxy(target, {
  get: (t: any, key: string): any => {
    if (key === "x") {
      return t[key] * 10;
    }
    return "missing:" + key;
  },
  set: (t: any, key: string, value: any): boolean => {
    log.push(key + "=" + value);
    t[key] = value;
    return true;
  },
});
console.log(p.x); // 10
console.log(p.z); // missing:z
p.y = 2;
console.log(target.y); // 2
console.log(log.join(",")); // y=2
