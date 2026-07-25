console.log([1, 2, 3].some((x, i) => i === 2 && x === 3));
console.log([1, 2, 3].every((x, i) => x > i));
[10, 20, 30].forEach((x, i) => console.log(`${i}=${x}`));
console.log([5, 6, 7].find((x, i) => i === 1));
console.log([5, 6, 7].findIndex((x, i) => x === 7 && i === 2));
console.log([5, 6, 7].findLast((x, i) => i < 2));
console.log([5, 6, 7].findLastIndex((x, i) => x > 5));
