// Phase 3 item 66 sub-slice: an object method whose body is more than a single return
// statement now boxes into a dynamic value, so the coercion protocol can invoke it. Only
// a single-return body boxed before; a body with locals or a branch handed back. valueOf,
// toString, and [Symbol.toPrimitive] are the callers the runtime drives when an object
// crosses into a primitive context, and each now lowers a multi-statement body through the
// same value.NewFunc closure the single-return form already used.

// A multi-statement valueOf drives the relational and equality protocols.
const counter: any = {
  valueOf() {
    let a = 5;
    let b = 2;
    return a * b;
  },
};
console.log(counter < 100);
console.log(counter == 10);

// A multi-statement toString drives string coercion.
const label: any = {
  toString() {
    let head = "he";
    let tail = "llo";
    return head + tail;
  },
};
console.log(`${label}`);

// A multi-statement [Symbol.toPrimitive] with a branch drives numeric coercion.
const picked: any = {
  [Symbol.toPrimitive]() {
    let n = 7;
    if (n > 3) {
      return n;
    }
    return 0;
  },
};
console.log(picked + 1);
