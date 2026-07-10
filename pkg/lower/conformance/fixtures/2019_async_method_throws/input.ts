class Box {
  async boom(): Promise<number> {
    throw new Error("bang");
    return 0;
  }
}
console.log("start");
new Box().boom().catch(e => console.log("caught:" + e.message));
console.log("end");
