// The numeric unary operators over a dynamic operand coerce through ToNumber
// before they apply. Minus negates the resulting float64, plus is the coercion
// and nothing else, and bitwise not narrows to a 32-bit integer, complements,
// and widens back. A numeric string and a boolean both coerce, so -"5" is -5,
// +true is 1, and ~5 is -6.
function neg(x: any): number { return -x; }
function pos(x: any): number { return +x; }
function bnot(x: any): number { return ~x; }

console.log(neg("5"));
console.log(neg(true));
console.log(neg(3.5));
console.log(pos("5"));
console.log(pos(true));
console.log(bnot(5));
console.log(bnot(0));
console.log(bnot("5"));
