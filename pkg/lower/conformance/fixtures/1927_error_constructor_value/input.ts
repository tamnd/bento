// A built-in error constructor named as a value boxes to a first-class value that
// carries its name, compares equal to itself, and reports typeof "function". This
// is the shape assert.throws relies on: it takes an error constructor as an
// argument, reads its name for the failure message, and compares two constructors
// for identity. The any-typed bindings force the boxed form the dynamic argument
// slot of assert.throws also forces.
let ctor: any = TypeError;
let ctorName: string = ctor.name;
console.log(ctorName);

let range: any = RangeError;
let rangeName: string = range.name;
console.log(rangeName);

let alsoType: any = TypeError;
let sameCtor: boolean = ctor === alsoType;
console.log(sameCtor);

let diffCtor: boolean = ctor === range;
console.log(diffCtor);

let syntax: any = SyntaxError;
let tag: string = typeof syntax;
console.log(tag);
