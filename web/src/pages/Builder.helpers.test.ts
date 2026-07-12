import { describe, expect, it } from "vitest";
import { fallsBackToDefault, resolveGeneratedName } from "./Builder.helpers";

describe("resolveGeneratedName", () => {
  it("returns the default when nothing is typed and no prefix is set", () => {
    expect(resolveGeneratedName(undefined, "plan", undefined)).toBe("plan");
    expect(resolveGeneratedName("", "plan", "")).toBe("plan");
  });

  it("prefixes a name left at its default", () => {
    expect(resolveGeneratedName(undefined, "plan", "af")).toBe("af-plan");
    expect(resolveGeneratedName("plan", "plan", "af")).toBe("af-plan");
    expect(resolveGeneratedName("  plan  ", "plan", " af ")).toBe("af-plan");
  });

  it("lets an explicit rename win verbatim, without the prefix", () => {
    expect(resolveGeneratedName("blueprint", "plan", "af")).toBe("blueprint");
    expect(resolveGeneratedName("blueprint", "plan", "")).toBe("blueprint");
  });

  it("treats an invalid rename as the default, so the prefix still applies", () => {
    // Backend's customName rejects these and falls back — the preview must not lie.
    expect(resolveGeneratedName("My Plan!", "plan", "af")).toBe("af-plan");
    expect(resolveGeneratedName("-plan", "plan", "")).toBe("plan");
  });

  it("ignores an invalid prefix, matching backend leniency", () => {
    expect(resolveGeneratedName(undefined, "plan", "AF!")).toBe("plan");
    expect(resolveGeneratedName(undefined, "plan", "-af")).toBe("plan");
  });
});

describe("fallsBackToDefault", () => {
  it("flags only non-empty invalid input", () => {
    expect(fallsBackToDefault("My Plan!")).toBe(true);
    expect(fallsBackToDefault("plan")).toBe(false);
    expect(fallsBackToDefault("")).toBe(false);
    expect(fallsBackToDefault(undefined)).toBe(false);
  });
});
