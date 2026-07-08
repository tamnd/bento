// o["k"] with a string-literal key is the bracket spelling of the dotted read
// o.k, so a key the fixed shape does not declare folds the same way: to the boxed
// undefined over the evaluated receiver. The checker reports such a read as an
// index error rather than a missing property ("expression of type '"k"' can't be
// used to index type 'Y'"), a diagnostic the AOT front door tolerates alongside
// the dotted one. A declared key still reads its Go field, a computed key stays a
// later slice, and a receiver that carries an effect still runs exactly once.

const point = { x: 1, label: "p" };
console.log(point["x"]);
console.log(point["z"]);
console.log(String(point["z"]));

let calls = 0;
function make() {
  calls++;
  return { a: 10 };
}
console.log(make()["missing"]);
console.log(calls);
