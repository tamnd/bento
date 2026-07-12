const obj = { a: 1, secret: 2, b: 3 };
console.log(JSON.stringify(obj, (key, value) => {
  if (key === "secret") {
    return undefined;
  }
  return value;
}));
const whitelist = { a: 1, b: 2, c: 3 };
console.log(JSON.stringify(whitelist, ["a", "c"]));
console.log(JSON.stringify(whitelist, ["a", "c"], 2));
