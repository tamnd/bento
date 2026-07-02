export function sj(s: string, sep: string, glue: string): string {
  return s.split(sep).join(glue);
}
