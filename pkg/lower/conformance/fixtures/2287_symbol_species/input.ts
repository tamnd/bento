// Symbol.species lets a subclass of Array or a typed array choose the constructor its
// derived methods, map and filter and slice, build their result from. Honoring it
// needs a class that extends the built-in and a static species accessor the method
// path consults before it constructs. bento does not lower a class that extends a
// built-in like Array, so the heritage clause hands the class back before species can
// matter. Deriving from a built-in and honoring Symbol.species is a later slice.
class MyArray<T> extends Array<T> {
  static get [Symbol.species](): ArrayConstructor {
    return Array;
  }
}
const a = new MyArray<number>();
a.push(1, 2, 3);
const mapped = a.map((x) => x * 2);
console.log(mapped.length);
