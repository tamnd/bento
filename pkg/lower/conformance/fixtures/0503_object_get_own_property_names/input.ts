// Object.getOwnPropertyNames on a fixed-shape object returns its own property
// names in declaration order. On a plain object it matches Object.keys, since a
// struct shape has no non-enumerable or symbol keys for the two to differ over,
// so it lowers to the same compile-time name list rather than a runtime walk.
function run(): void {
  const o = { name: "hi", age: 3, active: true };
  const ks = Object.getOwnPropertyNames(o);
  console.log(o.name);
  console.log(ks.length);
  console.log(ks.join(","));

  // the names keep declaration order, not sorted order
  const scores = { z: 1, a: 2, m: 3 };
  console.log(scores.z);
  console.log(Object.getOwnPropertyNames(scores).join(","));
}

run();
