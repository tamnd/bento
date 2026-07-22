function pt([x, y]: [number, number]): number {
  return x + y;
}
console.log(pt([3, 4]) + "");

function greet([name, times]: [string, number]): string {
  let out = "";
  for (let i = 0; i < times; i++) {
    out = out + name;
  }
  return out;
}
console.log(greet(["ab", 3]));

function firstOnly([head]: [string, boolean]): string {
  return head;
}
console.log(firstOnly(["only", true]));
