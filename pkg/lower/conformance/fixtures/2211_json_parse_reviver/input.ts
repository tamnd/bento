const doubled = JSON.parse('{"a":1,"b":2}', (key, value) => {
  if (typeof value === "number") {
    return value * 2;
  }
  return value;
});
console.log(JSON.stringify(doubled));

const pruned = JSON.parse('{"secret":1,"keep":2}', (key, value) => {
  if (key === "secret") {
    return undefined;
  }
  return value;
});
console.log(JSON.stringify(pruned));

const arr = JSON.parse('[1,2,3]', (key, value) => {
  if (typeof value === "number") {
    return value * 2;
  }
  return value;
});
console.log(JSON.stringify(arr));

const nested = JSON.parse('{"n":{"x":10}}', (key, value) => {
  if (typeof value === "number") {
    return value * 2;
  }
  return value;
});
console.log(JSON.stringify(nested));
