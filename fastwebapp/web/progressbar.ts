// Determinate ("37% uploaded") and indeterminate ("working, no known duration") progress. Two
// modes because they mean different things to a user - a determinate bar promises an ETA an
// indeterminate one can't, so they're not the same widget with a flag, they're a mode switch.
export function clampPercent(value: number): number {
  if (Number.isNaN(value)) return 0;
  return Math.min(100, Math.max(0, value));
}

export class ProgressBar {
  readonly el: HTMLElement;
  private readonly track: HTMLElement;
  private readonly fill: HTMLElement;
  private mode: "determinate" | "indeterminate" = "determinate";

  constructor(container: HTMLElement) {
    this.el = document.createElement("div");
    this.el.className = "fwa-progressbar";
    this.track = document.createElement("div");
    this.track.className = "fwa-progressbar-track";
    this.fill = document.createElement("div");
    this.fill.className = "fwa-progressbar-fill";
    this.track.append(this.fill);
    this.el.append(this.track);
    container.append(this.el);
    this.set(0);
  }

  // Sets a known percentage (0-100, clamped) and switches to determinate mode if it wasn't.
  set(percent: number): void {
    this.mode = "determinate";
    this.el.classList.remove("fwa-progressbar-indeterminate");
    this.fill.style.width = `${clampPercent(percent)}%`;
  }

  // Switches to indeterminate mode - a CSS animation on fwa-progressbar-indeterminate drives
  // the visual, this just toggles the class.
  setIndeterminate(): void {
    this.mode = "indeterminate";
    this.el.classList.add("fwa-progressbar-indeterminate");
  }

  get currentMode(): "determinate" | "indeterminate" {
    return this.mode;
  }
}
