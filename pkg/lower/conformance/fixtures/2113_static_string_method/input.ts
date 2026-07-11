class Reg {
  static "look up"(): number {
    return 3;
  }
  static ["find"](): number {
    return 4;
  }
}
console.log(String(Reg["look up"]() + Reg["find"]()));
