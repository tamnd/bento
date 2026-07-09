// ECMAScript lets an identifier carry unicode escapes, so \u0061bc and abc name
// the same binding and a program is free to declare it one way and read it the
// other. The lowerer decodes the escape to the name it denotes before mangling,
// so the two spellings collapse to one Go identifier that a declaration and
// every reference share. The test262 identifiers suite spells names this way
// throughout, so a mismatch here refuses to compile the emitted Go.
const \u0061bc = 3;
const obj = { \u0064: 4 };
console.log(abc + obj.d);
