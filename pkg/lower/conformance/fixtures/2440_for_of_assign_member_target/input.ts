const o = { last: 0, tail: "" };
const nums = [1, 2, 3];
for (o.last of nums) {
  console.log(o.last);
}
const words = ["a", "b", "c"];
for (o.tail of words) {
  console.log(o.tail);
}
for (o.tail of "hi") {
  console.log(o.tail);
}
console.log("last " + o.last + o.tail);
