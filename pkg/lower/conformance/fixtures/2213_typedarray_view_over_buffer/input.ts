const buf = new ArrayBuffer(16);

const whole = new Int32Array(buf);
console.log(whole.length);

const tail = new Int32Array(buf, 8);
console.log(tail.length);

const two = new Int32Array(buf, 4, 2);
console.log(two.length);

const bytes = new Uint8Array(buf, 2, 4);
console.log(bytes.length);
