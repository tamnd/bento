// Write a batch of small files, read them all back, then delete them. This is a
// syscall-bound workload that leans on the runtime's node:fs layer rather than
// its JavaScript engine, so it shows the cost of the host bridge.
import { mkdtempSync, writeFileSync, readFileSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

const dir = mkdtempSync(join(tmpdir(), "bento-bench-"));
const count = 200;
const payload = "x".repeat(512);

let bytes = 0;
for (let pass = 0; pass < 3; pass++) {
  for (let i = 0; i < count; i++) {
    writeFileSync(join(dir, "f" + i + ".txt"), payload);
  }
  for (let i = 0; i < count; i++) {
    bytes += readFileSync(join(dir, "f" + i + ".txt"), "utf8").length;
  }
}
rmSync(dir, { recursive: true, force: true });
console.log(bytes);
