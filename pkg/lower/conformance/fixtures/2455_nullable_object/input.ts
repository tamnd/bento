function describe(box: { a: number } | null): string {
  if (box === null) return "none";
  return "a=" + box.a;
}

function describeOpt(box: { a: number } | null | undefined): string {
  if (box === null) return "null";
  if (box === undefined) return "undef";
  return "a=" + box.a;
}

let slot: { a: number } | null = { a: 1 };
console.log(describe(slot));
if (slot !== null) {
  console.log(slot.a);
}
slot = null;
console.log(describe(slot));
if (slot === null) {
  console.log("cleared");
}

let opt: { a: number } | null | undefined = { a: 2 };
console.log(describeOpt(opt));
opt = undefined;
console.log(describeOpt(opt));
opt = null;
console.log(describeOpt(opt));
