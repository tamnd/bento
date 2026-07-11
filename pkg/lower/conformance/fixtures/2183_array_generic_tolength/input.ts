const frac: any = {};
frac[0] = "a";
frac[1] = "b";
frac[2] = "c";
frac.length = 2.9;
const i: any = Array.prototype.indexOf.call(frac, "c");
console.log(i);

const neg: any = {};
neg[0] = "x";
neg.length = -5;
const j: any = Array.prototype.includes.call(neg, "x");
console.log(j);

const str: any = {};
str[0] = "y";
str[1] = "z";
str.length = "2";
const k: any = Array.prototype.indexOf.call(str, "z");
console.log(k);
