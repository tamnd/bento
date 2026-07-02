const width = 5;
let total = 0;
for (let i = 0; i < width; i++) {
  total += i;
}
process.stdout.write(String(total) + "\n");
