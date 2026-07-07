// A compound += on an any-typed target concatenates a string onto a boxed value.
// value.Concat returns a bstr, not a box, so the result must wrap back into a
// value.Value to fit the dynamic slot. assert.throws and assert.sameValue in the
// test262 prelude take this shape: message is any and they append a separator with
// message += ' ' before raising. A numeric += on the same target still boxes
// through value.Add and needs no extra wrap.
function label(message: any): string {
  message += " world";
  return message;
}
function bump(x: any): string {
  x += 5;
  return "n=" + x;
}
console.log(label("hi"));
console.log(bump(37));
