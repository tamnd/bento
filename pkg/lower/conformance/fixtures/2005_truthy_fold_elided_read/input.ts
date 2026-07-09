// An object in a boolean position is always truthy, so lowerTruthy folds the
// condition to the Go constant true and never lowers the read. When the object's
// only read is that condition, the binding would be declared and not used and the
// Go would not build. countElidedReads records the folded condition so the binding
// gets a trailing blank. test262 reaches this with for (; obj; ) over a plain
// object, where the loop breaks after one pass.
var accessed = false;
var obj = { value: false };
for (var i = 0; obj; ) {
  accessed = true;
  break;
}
console.log(accessed);
