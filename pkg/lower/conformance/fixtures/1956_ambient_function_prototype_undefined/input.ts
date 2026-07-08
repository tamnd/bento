// A read of .prototype on a built-in function that is not a constructor
// (isFinite, isNaN, parseInt, parseFloat) is undefined: these functions carry
// no prototype property. Bento models such a function only as a call target,
// so a bare reference has no Go value and a naive lowering emits a selector on
// a Go type name that does not build. The read folds to the undefined singleton
// the language yields, and the strict compare against undefined holds.
console.log(isFinite.prototype === undefined);
console.log(isNaN.prototype === undefined);
console.log(parseInt.prototype === undefined);
console.log(parseFloat.prototype === undefined);
