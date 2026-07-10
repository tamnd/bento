class C {
  "my method"(): number { return 42; }
  "my field": number = 5;
  static "of"(): number { return 9; }
  static "s field": number = 8;
  get "x y"(): number { return 3; }
}
const c = new C();
console.log(String(c["my method"]()));
console.log(String(c["my field"]));
