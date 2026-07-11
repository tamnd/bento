// A generator's object surface driven two ways over the same generator function. A
// bare for...of pulls it to exhaustion, and a hand-rolled loop over next() walks it a
// step at a time, reading .value and .done off each { value, done } result until done
// latches; both see the same values in the same order. The send form next(v) threads a
// value into the yield it resumes, which for...of cannot express because it always
// sends undefined, so echo's `yield 0` evaluates to the 7 the second next passes.
function* nums(): Generator<number> {
  yield 1;
  yield 2;
  yield 3;
}

for (const x of nums()) {
  console.log("of " + String(x));
}

const it = nums();
let r = it.next();
while (!r.done) {
  console.log("next " + String(r.value));
  r = it.next();
}

function* echo(): Generator<number, void, number> {
  const got = yield 0;
  console.log("got " + String(got));
}

const e = echo();
e.next();
e.next(7);
