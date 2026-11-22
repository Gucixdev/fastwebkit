import { describe, it, expect, vi } from "vitest";
import { NotificationCenter } from "./notifications";

describe("NotificationCenter", () => {
  it("notify() renders a toast with the message and type class", () => {
    const center = new NotificationCenter(document.createElement("div"));
    center.notify("saved", "success");

    const toast = center.el.querySelector(".fwa-toast")!;
    expect(toast.textContent).toBe("saved");
    expect(toast.classList.contains("fwa-toast-success")).toBe(true);
  });

  it("defaults to type 'info'", () => {
    const center = new NotificationCenter(document.createElement("div"));
    center.notify("hello");
    expect(center.el.querySelector(".fwa-toast")!.classList.contains("fwa-toast-info")).toBe(true);
  });

  it("dismiss() removes the notification from both state and the DOM", () => {
    const center = new NotificationCenter(document.createElement("div"));
    const id = center.notify("bye", "error", 0);
    expect(center.all).toHaveLength(1);

    center.dismiss(id);
    expect(center.all).toHaveLength(0);
    expect(center.el.querySelector(".fwa-toast")).toBeNull();
  });

  it("auto-dismisses after durationMs", () => {
    vi.useFakeTimers();
    const center = new NotificationCenter(document.createElement("div"));
    center.notify("auto", "info", 1000);
    expect(center.all).toHaveLength(1);

    vi.advanceTimersByTime(1000);
    expect(center.all).toHaveLength(0);
    vi.useRealTimers();
  });

  it("durationMs <= 0 means it never auto-dismisses", () => {
    vi.useFakeTimers();
    const center = new NotificationCenter(document.createElement("div"));
    center.notify("sticky", "info", 0);
    vi.advanceTimersByTime(100000);
    expect(center.all).toHaveLength(1);
    vi.useRealTimers();
  });

  it("clicking a toast dismisses it", () => {
    const center = new NotificationCenter(document.createElement("div"));
    center.notify("click me", "info", 0);
    center.el.querySelector(".fwa-toast")!.dispatchEvent(new Event("click"));
    expect(center.all).toHaveLength(0);
  });

  it("supports multiple simultaneous notifications", () => {
    const center = new NotificationCenter(document.createElement("div"));
    center.notify("one", "info", 0);
    center.notify("two", "info", 0);
    expect(center.el.querySelectorAll(".fwa-toast")).toHaveLength(2);
  });
});
