// A var declared without an initializer lives in a value.Value box. A later
// count = 0 narrows the checker's type to number, so at count++ the checker types
// count number even though the storage is still the box. Emitting a Go count++
// over a value.Value would not build, so the update routes through value.Inc on
// the box, and value.Dec for the decrement. test262 reaches this with the for-head
// counter tests that declare the counter with a bare var before the loop.
var count;
count = 0;
count++;
count++;
console.log(count);

var down;
down = 5;
down--;
console.log(down);
