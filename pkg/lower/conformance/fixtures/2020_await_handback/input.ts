class W {
  async wait(): Promise<number> {
    const v = await Promise.resolve(1);
    return v;
  }
}
new W().wait();
