import { describe, it, expect, vi, afterEach } from "vitest";
import { ConstraintLayout, Operator, Strength } from "./constraint-layout";

// ResizeObserver isn't implemented in jsdom, so these tests drive the solver directly via
// solve() rather than through observeResize() - the constraint math is what's under test, not
// the browser's resize plumbing.
describe("ConstraintLayout", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("positions two boxes side by side filling the container", () => {
    const container = document.createElement("div");
    const sidebarEl = document.createElement("div");
    const mainEl = document.createElement("div");

    const layout = new ConstraintLayout(container);
    const sidebar = layout.addBox(sidebarEl);
    const main = layout.addBox(mainEl);

    layout.solver.suggestValue(layout.container.width, 1000);
    layout.solver.suggestValue(layout.container.height, 500);

    layout.constrain(sidebar.left, Operator.Eq, 0);
    layout.constrain(sidebar.top, Operator.Eq, 0);
    layout.constrain(main.top, Operator.Eq, 0);
    layout.constrain(sidebar.height, Operator.Eq, layout.container.height);
    layout.constrain(main.height, Operator.Eq, layout.container.height);
    layout.constrain(main.left, Operator.Eq, sidebar.left.plus(sidebar.width));
    layout.constrain(sidebar.width, Operator.Eq, main.width.multiply(0.3), Strength.strong);
    layout.constrain(sidebar.width.plus(main.width), Operator.Eq, layout.container.width);

    layout.solve();

    expect(sidebarEl.style.left).toBe("0px");
    expect(mainEl.style.left).toBe(`${sidebar.width.value()}px`);
    expect(sidebar.height.value()).toBe(500);
    expect(main.height.value()).toBe(500);
    // sidebar is always exactly 0.3x of main's width
    expect(sidebar.width.value()).toBeCloseTo(main.width.value() * 0.3, 5);
    expect(sidebar.width.value() + main.width.value()).toBeCloseTo(1000, 5);
  });

  it("keeps the 0.3x relationship after the container is resized", () => {
    const container = document.createElement("div");
    const sidebarEl = document.createElement("div");
    const mainEl = document.createElement("div");

    const layout = new ConstraintLayout(container);
    const sidebar = layout.addBox(sidebarEl);
    const main = layout.addBox(mainEl);

    layout.constrain(sidebar.left, Operator.Eq, 0);
    layout.constrain(main.left, Operator.Eq, sidebar.left.plus(sidebar.width));
    layout.constrain(sidebar.width, Operator.Eq, main.width.multiply(0.3), Strength.strong);
    layout.constrain(sidebar.width.plus(main.width), Operator.Eq, layout.container.width);

    layout.solver.suggestValue(layout.container.width, 1000);
    layout.solver.suggestValue(layout.container.height, 500);
    layout.solve();
    const ratioBefore = sidebar.width.value() / main.width.value();

    layout.solver.suggestValue(layout.container.width, 2000);
    layout.solve();
    const ratioAfter = sidebar.width.value() / main.width.value();

    expect(ratioBefore).toBeCloseTo(0.3, 2);
    expect(ratioAfter).toBeCloseTo(0.3, 2);
  });

  it("does not force position:absolute on the container itself (the collapse bug)", () => {
    const container = document.createElement("div");
    container.style.height = "100vh";
    new ConstraintLayout(container);
    // only position:relative should be touched - absolute would strip it from flow and
    // collapse it to shrink-to-fit size, which was the original rendering bug.
    expect(container.style.position).toBe("relative");
    expect(container.style.height).toBe("100vh");
  });

  it("addBox does force position:absolute on child elements", () => {
    const container = document.createElement("div");
    const layout = new ConstraintLayout(container);
    const box = layout.addBox(document.createElement("div"));
    expect(box.el.style.position).toBe("absolute");
  });

  it("constrain() does not throw 'duplicate constraint' (the double-add bug)", () => {
    const layout = new ConstraintLayout(document.createElement("div"));
    const box = layout.addBox(document.createElement("div"));
    expect(() => layout.constrain(box.left, Operator.Eq, 0)).not.toThrow();
  });

  it("appends boxes into the container element", () => {
    const container = document.createElement("div");
    const layout = new ConstraintLayout(container);
    const el = document.createElement("div");
    layout.addBox(el);
    expect(container.contains(el)).toBe(true);
  });
});
