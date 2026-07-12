// subarray makes a new view over the same buffer, so it shares the bytes with the
// receiver: a write through either shows through the other, unlike slice, whose
// result owns a fresh copy.
const a = new Int32Array([1, 2, 3, 4, 5]);
const sub = a.subarray(1, 4);
console.log(sub.length);
console.log(sub.join(","));

// a write through the subarray shows in the parent view.
sub[0] = 99;
console.log(a.join(","));

// a write through the parent shows in the subarray.
a[3] = 88;
console.log(sub.join(","));

// the subarray starts at the parent's byte offset advanced by the start element.
console.log(sub.byteOffset);

// a negative bound counts from the end.
const tail = a.subarray(-2);
console.log(tail.join(","));
