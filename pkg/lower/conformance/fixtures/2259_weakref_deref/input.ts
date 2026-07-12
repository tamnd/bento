// A WeakRef holds a single object weakly and reads it back with deref. While the
// target is reachable, deref returns the same object every call, so a live target is
// stable and compares equal to the original by reference identity. Once the target is
// collected deref returns undefined instead, but that turn is not something a program
// can force, so this covers the live-target reads the operational surface guarantees.
function run(): void {
  const a = { id: 42 };
  const wr = new WeakRef(a);

  const o = wr.deref();
  if (o !== undefined) {
    console.log(o.id);
    console.log(o === a);
  } else {
    console.log("collected");
  }

  const o2 = wr.deref();
  console.log(o2 !== undefined);
}

run();
