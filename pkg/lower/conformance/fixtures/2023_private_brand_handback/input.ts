class C {
  #m(): number {
    return 1;
  }
  has(o: any): boolean {
    return #m in o;
  }
}
new C();
