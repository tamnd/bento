function tail([first, ...rest]: number[]): number {
  return first + rest.length;
}
console.log(tail([10, 20, 30, 40]));
console.log(tail([7]));
