console.log([1, 2, 3].map((x, i) => x + i).join(","));
console.log([10, 20, 30].map((x, i) => `${i}:${x}`).join(","));
console.log([1, 2, 3, 4].filter((x, i) => i % 2 === 0).join(","));
console.log(Array.from([1, 2, 3], (x, i) => x + i).join(","));
