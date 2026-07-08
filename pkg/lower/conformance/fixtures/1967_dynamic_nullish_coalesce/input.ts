// Nullish coalescing on a dynamic left lowers to value.Coalesce over both boxed
// operands. The runtime tests presence, not truthiness, so a present zero or
// empty string is kept while null and undefined fall to the fallback. That is
// the ?? contract the optional path already keeps, here over dynamic values.

function pick(x: any, fb: any): any {
  return x ?? fb;
}

// A present but falsy value is kept, the difference between ?? and ||.
console.log(pick(0, 99));
console.log(pick("", "z"));

// null and undefined are the two nullish values that fall to the fallback.
console.log(pick(null, 99));
console.log(pick(undefined, 7));

// A present truthy value is kept unchanged.
console.log(pick("kept", "z"));
