// A thrown boolean is a value like any other: uncaught, it reports its String form
// and exits non-zero. This covers the primitive-that-is-not-a-string arm of the
// carrier, so the number, boolean, null, and undefined cases share one path.
console.log("start");
throw true;
