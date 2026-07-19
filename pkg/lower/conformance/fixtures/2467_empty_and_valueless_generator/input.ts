function* empty(): Generator<number> {}

let count = 0;
for (const x of empty()) {
  count++;
}
console.log(count);
console.log(empty().next().done);

function* signals(): Generator<undefined> {
  yield;
  yield;
  yield;
}

let ticks = 0;
for (const _ of signals()) {
  ticks++;
}
console.log(ticks);
