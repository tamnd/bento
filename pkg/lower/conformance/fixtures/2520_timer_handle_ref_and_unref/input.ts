const late = setTimeout(() => {
  console.log("late");
}, 50000);
console.log("fresh", late.hasRef());
late.unref();
console.log("unrefed", late.hasRef());
setTimeout(() => {
  console.log("soon");
}, 1);
