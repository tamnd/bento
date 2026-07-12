// A has trap answers the in operator and a deleteProperty trap answers delete, so a
// proxy can report a property the target never held and can refuse a removal the
// target would otherwise perform.
const log: string[] = [];
const target: any = { real: 1 };
const p: any = new Proxy(target, {
  has: (t: any, key: string): boolean => key === "virtual" || key in t,
  deleteProperty: (t: any, key: string): boolean => {
    log.push("delete:" + key);
    return false;
  },
});
console.log("virtual" in p); // true
console.log("real" in p); // true
console.log("missing" in p); // false
console.log(delete p.real); // false
console.log(target.real); // 1
console.log(log.join(",")); // delete:real
