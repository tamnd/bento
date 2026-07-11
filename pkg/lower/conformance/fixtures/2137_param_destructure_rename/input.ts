function label({ name: n, id: i }: { name: string; id: number }): string {
  return n + ":" + i;
}
console.log(label({ name: "sam", id: 7 }));
