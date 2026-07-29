import * as os from "node:os";
import { arch, freemem, totalmem } from "node:os";

console.log(os.platform().length > 0);
console.log(os.arch() === arch());
console.log(os.type().length > 0);
console.log(os.release().length > 0);
console.log(os.version().length > 0);
console.log(os.machine().length > 0);
console.log(os.hostname().length > 0);
console.log(os.homedir().length > 0);
console.log(os.endianness() === "LE" || os.endianness() === "BE");
console.log(totalmem() > 0);
console.log(freemem() > 0 && freemem() <= totalmem());
console.log(os.uptime() > 0);
console.log(os.availableParallelism() >= 1);
