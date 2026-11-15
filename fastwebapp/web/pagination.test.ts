import { describe, it, expect } from "vitest";
import { pageCount, clampPage, paginate, Pagination } from "./pagination";

describe("pageCount", () => {
  it("rounds up", () => {
    expect(pageCount(21, 10)).toBe(3);
    expect(pageCount(20, 10)).toBe(2);
  });
  it("is at least 1 even for zero items", () => {
    expect(pageCount(0, 10)).toBe(1);
  });
  it("is 0 for an invalid page size", () => {
    expect(pageCount(10, 0)).toBe(0);
  });
});

describe("clampPage", () => {
  it("clamps below 1", () => {
    expect(clampPage(0, 5)).toBe(1);
    expect(clampPage(-3, 5)).toBe(1);
  });
  it("clamps above the total", () => {
    expect(clampPage(99, 5)).toBe(5);
  });
  it("passes through in-range pages", () => {
    expect(clampPage(3, 5)).toBe(3);
  });
});

describe("paginate", () => {
  const items = Array.from({ length: 23 }, (_, i) => i + 1);

  it("returns the right slice for a middle page", () => {
    expect(paginate(items, 2, 10)).toEqual([11, 12, 13, 14, 15, 16, 17, 18, 19, 20]);
  });
  it("returns a partial last page", () => {
    expect(paginate(items, 3, 10)).toEqual([21, 22, 23]);
  });
  it("clamps an out-of-range page instead of returning nothing", () => {
    expect(paginate(items, 99, 10)).toEqual([21, 22, 23]);
  });
});

describe("Pagination", () => {
  it("renders one button per page plus prev/next, marking the current page", () => {
    const p = new Pagination(document.createElement("div"));
    p.update(25, 10); // 3 pages

    const buttons = Array.from(p.el.querySelectorAll("button"));
    expect(buttons).toHaveLength(5); // prev, 1, 2, 3, next
    expect(buttons[1]!.classList.contains("fwa-pagination-active")).toBe(true);
    expect((buttons[0] as HTMLButtonElement).disabled).toBe(true); // prev disabled on page 1
  });

  it("clicking a page button updates currentPage and notifies listeners", () => {
    const p = new Pagination(document.createElement("div"));
    p.update(25, 10);
    const seen: number[] = [];
    p.onChange((page) => seen.push(page));

    const pageTwoButton = Array.from(p.el.querySelectorAll("button")).find((b) => b.textContent === "2")!;
    pageTwoButton.dispatchEvent(new Event("click"));

    expect(p.currentPage).toBe(2);
    expect(seen).toEqual([2]);
  });

  it("re-clamps the current page if the item count shrinks", () => {
    const p = new Pagination(document.createElement("div"));
    p.update(25, 10);
    const lastButton = Array.from(p.el.querySelectorAll("button")).find((b) => b.textContent === "3")!;
    lastButton.dispatchEvent(new Event("click"));
    expect(p.currentPage).toBe(3);

    p.update(5, 10); // now only 1 page
    expect(p.currentPage).toBe(1);
  });
});
