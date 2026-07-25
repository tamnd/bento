console.log([1, 2, 3, 4].reduce((acc, x, i) => acc + x * i, 0));
console.log([1, 2, 3, 4].reduce((acc, x, i) => acc + x * i));
console.log(["a", "b", "c"].reduceRight((acc, x, i) => acc + x + i, ""));
console.log([1, 2, 3, 4].reduceRight((acc, x, i) => acc + x * i));
console.log(["a", "b", "c"].reduce((acc, x, i) => `${acc}${i}:${x};`, ""));
