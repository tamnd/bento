// Optional chaining reads a member only when the receiver is present and
// short-circuits the whole chain to undefined otherwise, so ?? then supplies the
// fallback. A single link u?.name reads the field off a present user and yields
// undefined for a missing one. A multi-link u?.city?.name reads through only when
// every link before it is present, so one nullish receiver anywhere makes the
// whole result undefined without ever touching the later members.
interface City {
  name: string;
  population: number;
}
interface User {
  name: string;
  city: City;
}

function userName(u: User | undefined): string {
  return u?.name ?? "nobody";
}

function cityName(u: User | undefined): string {
  return u?.city?.name ?? "unknown";
}

function population(u: User | undefined): number {
  return u?.city?.population ?? -1;
}

function run(): void {
  const u: User = { name: "Ada", city: { name: "London", population: 9 } };
  console.log(userName(u));
  console.log(userName(undefined));
  console.log(cityName(u));
  console.log(cityName(undefined));
  console.log(population(u));
  console.log(population(undefined));
}

run();
