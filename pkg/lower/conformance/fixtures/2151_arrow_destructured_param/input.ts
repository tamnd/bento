const area = ({ w, h }: { w: number; h: number }): number => w * h;
const diff = ([x, y]: number[]): number => x - y;
console.log(area({ w: 3, h: 4 }));
console.log(diff([9, 4]));
