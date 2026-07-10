try {
  throw "stuff1";
} catch (a) {
  {
    let a = 3;
    console.log(String(a));
  }
}
