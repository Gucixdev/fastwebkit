import { describe, it, expect, beforeEach } from "vitest";
import { nextTheme, readStoredTheme, storeTheme, ThemeController } from "./theme";

describe("nextTheme", () => {
  it("cycles forward through the list", () => {
    expect(nextTheme("light", ["light", "dark"])).toBe("dark");
  });
  it("wraps around at the end", () => {
    expect(nextTheme("dark", ["light", "dark"])).toBe("light");
  });
  it("returns the current value unchanged when the list is empty", () => {
    expect(nextTheme("light", [])).toBe("light");
  });
});

describe("storeTheme / readStoredTheme", () => {
  beforeEach(() => localStorage.clear());

  it("round-trips through localStorage", () => {
    storeTheme("dark");
    expect(readStoredTheme()).toBe("dark");
  });

  it("returns null when nothing is stored", () => {
    expect(readStoredTheme()).toBeNull();
  });
});

describe("ThemeController", () => {
  beforeEach(() => {
    localStorage.clear();
    document.documentElement.removeAttribute("data-theme");
  });

  it("applies the default theme and sets data-theme on <html>", () => {
    new ThemeController(["light", "dark"], "light");
    expect(document.documentElement.getAttribute("data-theme")).toBe("light");
  });

  it("restores a previously stored theme instead of the default", () => {
    storeTheme("dark");
    const controller = new ThemeController(["light", "dark"], "light");
    expect(controller.theme).toBe("dark");
    expect(document.documentElement.getAttribute("data-theme")).toBe("dark");
  });

  it("set() persists, applies, and notifies listeners", () => {
    const controller = new ThemeController(["light", "dark"], "light");
    const seen: string[] = [];
    controller.onChange((t) => seen.push(t));

    controller.set("dark");

    expect(controller.theme).toBe("dark");
    expect(document.documentElement.getAttribute("data-theme")).toBe("dark");
    expect(readStoredTheme()).toBe("dark");
    expect(seen).toEqual(["dark"]);
  });

  it("cycle() advances to the next available theme", () => {
    const controller = new ThemeController(["light", "dark"], "light");
    controller.cycle();
    expect(controller.theme).toBe("dark");
    controller.cycle();
    expect(controller.theme).toBe("light");
  });
});
