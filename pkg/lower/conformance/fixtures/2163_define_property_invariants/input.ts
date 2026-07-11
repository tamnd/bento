const o: any = {};
Object.defineProperty(o, "a", { value: 1 });
try {
  Object.defineProperty(o, "a", { value: 2 });
  console.log("no throw");
} catch (e: any) {
  console.log(e.name);
}
console.log(o.a);
