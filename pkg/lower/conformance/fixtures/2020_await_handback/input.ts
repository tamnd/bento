class W {
  async wait(v: any): Promise<number> {
    const x = await v;
    return x;
  }
}
new W().wait(1);
