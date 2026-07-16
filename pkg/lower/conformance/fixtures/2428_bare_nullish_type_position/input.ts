// A type position spelled exactly undefined, null, void, or never carries only a
// sentinel or nothing at all, yet a Go declaration still needs a type for the slot.
// Each lowers to value.Value, the boxed representation the undefined and null
// literals already take, so an array element, a struct field, and a tuple element
// typed this way all declare and lower. This covers a bare undefined and null array
// element, a null and a void struct field beside a real number field, a never[]
// element type (the type of an empty array literal), and a tuple whose second
// element is null. Reading the sentinel back and stringifying it is a separate
// slice, so each slot is proven through its length or its sibling number, the facts
// that exist only because the nullish slot itself lowered.

// An array whose element type is exactly undefined holds the undefined sentinel, and
// its length is the ordinary array length.
const undefs: undefined[] = [undefined, undefined, undefined];
console.log(undefs.length);

// An array of exactly null lowers the same way; its length counts the sentinels.
const nulls: null[] = [null, null];
console.log(nulls.length);

// A struct field typed null and one typed void take value.Value beside an ordinary
// number field, and the number field reads back unchanged only because the whole
// struct lowered.
type Slot = { tag: null; note: void; n: number };
const s: Slot = { tag: null, note: undefined, n: 7 };
console.log(s.n);

// An empty array literal has element type never, so its declared element lowers to
// the boxed placeholder; the array is simply empty.
const empty: never[] = [];
console.log(empty.length);

// A tuple whose second element is null pairs a real number with the null sentinel,
// and the number element reads back at its position.
const pair: [number, null] = [3, null];
console.log(pair[0]);
