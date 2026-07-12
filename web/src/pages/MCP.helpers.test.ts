import { describe, expect, it } from "vitest";
import { parseArgs, parseEnvLines, parseHeaderLines } from "./MCP.helpers";

describe("parseArgs", () => {
  it("splits on whitespace and keeps quoted segments whole", () => {
    expect(parseArgs('-y @modelcontextprotocol/server-github "two words"')).toEqual([
      "-y",
      "@modelcontextprotocol/server-github",
      "two words",
    ]);
    expect(parseArgs("")).toEqual([]);
    expect(parseArgs("  ")).toEqual([]);
  });
});

describe("parseEnvLines", () => {
  it("splits KEY=value per line, keeping later equals inside the value", () => {
    expect(parseEnvLines("API_TOKEN=${GH_TOKEN}\nOPTS=a=b\n\n  \n")).toEqual({
      API_TOKEN: "${GH_TOKEN}",
      OPTS: "a=b",
    });
  });

  it("skips lines without a key or separator", () => {
    expect(parseEnvLines("=nope\njustaword\nOK=1")).toEqual({ OK: "1" });
  });
});

describe("parseHeaderLines", () => {
  it("splits on the first colon so URLs in values survive", () => {
    expect(parseHeaderLines("Authorization: Bearer ${KEY}\nX-Ref: https://a:1")).toEqual({
      Authorization: "Bearer ${KEY}",
      "X-Ref": "https://a:1",
    });
  });
});
