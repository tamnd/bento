// A FinalizationRegistry registers a target with a held value and an optional
// unregister token, and calls its cleanup callback with the held value after the
// target is collected. The operational surface is register and unregister: register
// records the target, and unregister with the token removes the registration and
// reports whether it removed one, so a second unregister of the same token is false.
// The cleanup callback only runs after a collection, whose exact turn a program cannot
// force, so this covers the register and unregister results the surface guarantees.
function run(): void {
  const registry = new FinalizationRegistry<string>((held: string) => {
    console.log("cleanup " + held);
  });

  const token = { t: 1 };
  const a = { id: 1 };
  const b = { id: 2 };

  registry.register(a, "a", token);
  registry.register(b, "b");

  console.log(registry.unregister(token));
  console.log(registry.unregister(token));
}

run();
