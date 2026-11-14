import { describe, it, expect } from "vitest";
import { gridColumnsFor, gridAreasFor, ClassicLayout } from "./classic-layout";

describe("gridColumnsFor", () => {
  it("gives a single column when there is no sidebar", () => {
    expect(gridColumnsFor(1200, {}, false)).toBe("1fr");
  });

  it("puts a fixed-width sidebar beside main above the collapse breakpoint", () => {
    expect(gridColumnsFor(1200, { sidebarWidth: "240px", collapseBelow: 768 }, true)).toBe("240px 1fr");
  });

  it("collapses to a single column below the breakpoint", () => {
    expect(gridColumnsFor(500, { collapseBelow: 768 }, true)).toBe("1fr");
  });

  it("defaults sidebarWidth and collapseBelow when omitted", () => {
    expect(gridColumnsFor(1024, {}, true)).toBe("240px 1fr");
    expect(gridColumnsFor(767, {}, true)).toBe("1fr");
  });
});

describe("gridAreasFor", () => {
  it("stacks header/main/footer with no sidebar", () => {
    expect(gridAreasFor(false)).toBe('"header" "main" "footer"');
  });

  it("puts sidebar beside main, under the header, above the footer", () => {
    expect(gridAreasFor(true)).toBe('"header header" "sidebar main" "footer footer"');
  });
});

describe("ClassicLayout", () => {
  it("mounts header/sidebar/main/footer into the container in order and assigns grid areas", () => {
    const container = document.createElement("div");
    const header = document.createElement("header");
    const sidebar = document.createElement("nav");
    const main = document.createElement("main");
    const footer = document.createElement("footer");

    new ClassicLayout(container, { header, sidebar, main, footer });

    expect(Array.from(container.children)).toEqual([header, sidebar, main, footer]);
    expect(header.style.gridArea).toBe("header");
    expect(sidebar.style.gridArea).toBe("sidebar");
    expect(main.style.gridArea).toBe("main");
    expect(footer.style.gridArea).toBe("footer");
    expect(container.classList.contains("fwa-classic-layout")).toBe(true);
  });

  it("works without an optional sidebar or footer", () => {
    const container = document.createElement("div");
    const header = document.createElement("header");
    const main = document.createElement("main");

    new ClassicLayout(container, { header, main });

    expect(Array.from(container.children)).toEqual([header, main]);
  });

  it("injects its base stylesheet only once even if constructed twice", () => {
    const doc = document;
    const before = doc.querySelectorAll("#fastwebapp-classic-layout-style").length;
    new ClassicLayout(document.createElement("div"), {
      header: document.createElement("header"),
      main: document.createElement("main"),
    });
    new ClassicLayout(document.createElement("div"), {
      header: document.createElement("header"),
      main: document.createElement("main"),
    });
    const after = doc.querySelectorAll("#fastwebapp-classic-layout-style").length;
    expect(after - before).toBeLessThanOrEqual(1);
  });
});
