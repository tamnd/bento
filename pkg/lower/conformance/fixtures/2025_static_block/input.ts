// A static initialization block runs its statements at class-definition time,
// in member order, with the static fields in scope. It lowers into a package
// function the main body calls at the class declaration's position, so a read
// after the declaration observes the values the block wrote.
class Registry {
  static count: number = 0;
  static label: string = "";
  static {
    Registry.count = 3;
    Registry.label = "ready";
  }
  static {
    Registry.count = Registry.count + 1;
  }
}

console.log(String(Registry.count));
console.log(Registry.label);
