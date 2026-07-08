let out = 0;
block: {
  if (out === 0) {
    out = 1;
    break block;
  }
  out = 2;
}
console.log(out);

let found = -1;
search: {
  for (let i = 0; i < 3; i++) {
    if (i === 2) {
      found = i;
      break search;
    }
  }
  found = 99;
}
console.log(found);

const log: number[] = [];
blk: {
  for (let i = 0; i < 3; i++) {
    if (i === 1) break;
    log.push(i);
  }
  log.push(9);
  break blk;
  log.push(100);
}
console.log(log.join(","));
