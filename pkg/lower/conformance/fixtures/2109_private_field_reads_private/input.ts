class Rect {
  #width: number = 5;
  #height: number = 3;
  #area: number = this.#width * this.#height;
  area(): number {
    return this.#area;
  }
}
const r = new Rect();
console.log(r.area());
