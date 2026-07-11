const proto: any = { shared: 10, shadowed: 20 };
const child: any = Object.create(proto);
child.own = 40;
child.shadowed = 30;
console.log(child.shared);
console.log(child.shadowed);
console.log(child.own);
console.log(child.missing);
console.log("shared" in child);

const bare: any = Object.create(null);
bare.x = 5;
console.log(bare.x);
console.log("shared" in bare);
