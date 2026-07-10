var value: any = undefined;
value ??= function() {};
console.log(value.name);
var fn: any = 0;
fn ||= () => {};
console.log(fn.name);
