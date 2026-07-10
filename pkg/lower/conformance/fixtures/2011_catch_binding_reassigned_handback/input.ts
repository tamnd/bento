{
  let a = 3;
  try {
    throw "stuff2";
  } catch (a) {
    a = 4;
    console.log(String(a));
  }
  console.log(String(a));
}
