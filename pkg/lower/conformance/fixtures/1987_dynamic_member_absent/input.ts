// A read of a property a fixed-shape object does not declare is a provable miss:
// the shape interns to a Go struct with exactly its fields, so the property has
// nowhere to live and the language answers undefined. The checker flags each read
// ("Property 'X' does not exist on type 'Y'", with or without a spelling
// suggestion), a diagnostic the AOT front door tolerates so the read lowers to the
// boxed undefined over its evaluated receiver rather than gating the build. A
// receiver that carries an effect still runs exactly once, a declared property
// still reads its Go field, and the missing read coerces the way undefined does.

const point = { x: 1, y: 2, label: "p" };
console.log(point.z);
console.log(point.labl);
console.log(point.x);

let calls = 0;
function make() {
  calls++;
  return { a: 10 };
}
console.log(make().missing);
console.log(calls);
console.log(String(point.z));
