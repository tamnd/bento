// Promise.race settles the way the first input to settle does, so a fulfilled input
// ahead of a rejected one fulfills the race. Promise.any skips rejections and fulfills
// with the first input that fulfills, and only when every input rejects does it reject
// with an AggregateError whose errors array carries the reasons in input order, read
// here through name and the two dynamic indices.
const first: Promise<number>[] = [Promise.resolve(1), Promise.reject("no")];
Promise.race(first).then((v) => console.log("race:" + v));

const slow: Promise<number>[] = [Promise.reject("a"), Promise.reject("b")];
Promise.any(slow).catch((e) => {
  console.log("any-name:" + e.name);
  console.log("any-errors:" + e.errors[0] + "," + e.errors[1]);
});

const win: Promise<number>[] = [Promise.reject("x"), Promise.resolve(5)];
Promise.any(win).then((v) => console.log("any-ok:" + v));

console.log("sync");
