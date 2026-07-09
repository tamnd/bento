// A `var` counter a for loop declares is scoped to the whole function, not the
// loop, so a later loop can reuse it by plain assignment. This is the shape the
// test262 array-index tests write: for (var i = 0; ...) {} then for (i = 0; ...)
// {} reading the same i. Emitting the first loop's counter as a Go loop-local
// would leave the second loop's assignment referencing an undeclared name, so the
// counter hoists to the scope top and both loops write the one shared binding.
var total = 0;
for (var i = 0; i < 5; i++) {
  total = total + i;
}
for (i = 0; i < 3; i++) {
  total = total + i;
}
console.log(total);
