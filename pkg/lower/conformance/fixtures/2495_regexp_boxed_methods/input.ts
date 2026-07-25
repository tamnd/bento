// A dynamic re.test and re.exec off an any-typed regexp match through the live regexp
// the box carries: test reports the match boolean, exec returns the match array with
// its captures and .index or null on a miss, and a global regexp advances its own
// lastIndex across calls because every method read binds the one regexp.
const r: any = /a(b+)/;
console.log(r.test("zabbbc"));
console.log(r.test("nope"));
const res = r.exec("zabbbc");
console.log(res[0]);
console.log(res[1]);
console.log(res.index);
console.log(r.exec("nope"));
const g: any = /a/g;
console.log(g.test("aaa"));
console.log(g.lastIndex);
console.log(g.test("aaa"));
console.log(g.lastIndex);
