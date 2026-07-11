class Box {
  value;
  label;
  constructor() {
    console.log("ctor sees " + this.value + "|" + this.label);
  }
}
const b = new Box();
console.log(b.value + "|" + b.label);
