class Config {
  host: string = "localhost";
  port: number = 8080;
}
const c = new Config();
console.log(c.host + ":" + c.port);
