// A presence test on an optional local the checker has narrowed away from its
// optional union, the shape a write leaves behind, still lowers. After n = 42 the
// checker knows n holds a number, so `n === undefined` no longer types n as the
// optional union, but the Go slot stays value.Opt[number] and holds Some, so the
// test reads the raw slot's IsUndefined, which answers false the way JavaScript's
// 42 === undefined does. A fresh optional never written still reads undefined, and
// the rewrite fires for a narrowed optional parameter, whose field is the same slot.
function param(p: string | undefined): void {
  p = "written";
  if (p !== undefined) {
    console.log("p-defined");
  }
}

function run(): void {
  // A written optional number is never undefined.
  let n: number | undefined;
  n = 42;
  if (n === undefined) {
    console.log("n-undefined");
  } else {
    console.log("n-defined");
  }

  // A fresh optional string, never written, still reads undefined.
  let s: string | undefined;
  if (s === undefined) {
    console.log("s-undefined");
  }

  param(undefined);
}

run();
