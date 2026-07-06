// Class, method, and field names mangle through the same pure escape as
// functions and locals, so a declaration and every use agree on the one
// spelling: class $Counter becomes D_Counter, its field $count becomes
// D_count, its method $bump becomes D_bump, and NewD_Counter is the
// constructor the module speaks.
class $Counter {
  $count: number;
  constructor(start: number) {
    this.$count = start;
  }
  $bump(by: number): number {
    this.$count = this.$count + by;
    return this.$count;
  }
}

const $c = new $Counter(10);
console.log($c.$bump(5));
console.log($c.$bump(7));
console.log($c.$count);
