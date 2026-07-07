// An evolving array (`var a = []` grown by indexed writes) is declared any[], so
// its backing store holds value.Value, but control-flow analysis narrows a read
// a[i] to the element type the writes settle on. An arithmetic or concatenation
// use of the narrowed read needs the box unwrapped, so a[i] lowers to a.At(i)
// followed by the accessor the narrowed type names: AsNumber for a number read,
// AsString for a string read, AsBool for a boolean read. A test262
// exponentiation table drives this exact shape, `for (i) bases[i] ** exp`, where
// bases is grown one index at a time.
var nums = [];
nums[0] = 10;
nums[1] = 20;
var total = 0;
for (var i = 0; i < nums.length; i++) {
  total += nums[i];
}
console.log(total);

var strs = [];
strs[0] = "a";
strs[1] = "b";
var joined = "";
for (var j = 0; j < strs.length; j++) {
  joined += strs[j];
}
console.log(joined);

var flags = [];
flags[0] = true;
flags[1] = false;
var anyTrue = false;
for (var k = 0; k < flags.length; k++) {
  anyTrue = anyTrue || flags[k];
}
console.log(anyTrue);
