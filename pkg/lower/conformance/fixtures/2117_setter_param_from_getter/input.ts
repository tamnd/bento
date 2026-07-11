class Label {
  _text: string = "";
  get text(): string {
    return this._text;
  }
  set text(v) {
    this._text = v.toUpperCase();
  }
}
const l = new Label();
l.text = "bento";
console.log(l.text);
