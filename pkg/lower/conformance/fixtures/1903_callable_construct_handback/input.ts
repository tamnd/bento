// A callable object with a construct signature has no single Go func field to
// stand in for the value, so the whole shape hands back until a later slice
// models the construct side. The call-plus-properties shape above is what the
// prelude needs; this constructable variant is out of scope for now.
interface Factory {
    (name: string): void;
    label: string;
    new (name: string): object;
}

const make = function (name: string): void {
    console.log(name);
} as Factory;

make.label = "factory";
make("widget");
console.log(make.label);
