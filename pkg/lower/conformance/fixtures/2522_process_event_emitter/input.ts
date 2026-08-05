const seen: string[] = [];
function first(x: number) {
  seen.push("first " + x);
}
const second = (x: number) => {
  seen.push("second " + x);
};

process.on("ping", first);
process.on("ping", second);
console.log("count", process.listenerCount("ping"));
process.emit("ping", 1);
process.off("ping", second);
process.emit("ping", 2);
console.log("saved", process.listeners("ping").length);

process.once("pong", (x: number) => {
  seen.push("once " + x);
});
console.log("once", process.emit("pong", 3), process.emit("pong", 4));

process.prependListener("ping", (x: number) => {
  seen.push("pre " + x);
});
process.emit("ping", 5);

process.removeAllListeners("ping");
console.log("cleared", process.emit("ping", 6), process.listenerCount("ping"));

const tag = Symbol("tag");
process.on(tag, () => {
  seen.push("sym");
});
process.emit(tag);

console.log(seen.join("|"));
console.log("same", process.addListener === process.on, process.off === process.removeListener);
