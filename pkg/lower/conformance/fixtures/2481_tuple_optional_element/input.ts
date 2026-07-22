function label(pair: [number, string?]): string {
  if (pair[1] !== undefined) {
    return pair[0] + ":" + pair[1];
  }
  return pair[0] + ":none";
}

const a: [number, string?] = [1, "x"];
const b: [number, string?] = [2];
console.log(label(a));
console.log(label(b));
console.log(label([3, "y"]));
console.log(label([4]));
