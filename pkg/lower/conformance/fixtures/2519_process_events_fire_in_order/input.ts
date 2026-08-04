let n = 0;
process.on("exit", (code) => {
  console.log("exit", code);
});
process.on("beforeExit", () => {
  n = n + 1;
  console.log("beforeExit", n);
  if (n === 1) {
    setTimeout(() => {
      console.log("timer");
    }, 1);
  }
});
process.on("warning", () => {
  console.log("warning");
});
console.log("body");
