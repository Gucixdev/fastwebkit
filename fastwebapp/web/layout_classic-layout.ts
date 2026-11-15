// The layout most pages actually want: header on top, an optional sidebar, main content, a
// footer - the shape of most of the web, not the relational showcase in constraint-layout.ts.
// Plain CSS grid, no solver: there's nothing here a solver would do better, since "header full
// width, sidebar fixed width, content fills the rest" is exactly what grid-template-columns
// already expresses. Falls back to a single column below `collapseBelow` (sidebar moves above
// content) rather than disappearing, so the page still works on a phone.
export interface ClassicLayoutOptions {
  sidebarWidth?: string; // any valid CSS width, e.g. "240px"
  collapseBelow?: number; // viewport width in px below which the sidebar stacks above main
}

export interface ClassicLayoutSlots {
  header: HTMLElement;
  sidebar?: HTMLElement;
  main: HTMLElement;
  footer?: HTMLElement;
}

const STYLE_ID = "fastwebapp-classic-layout-style";

// Pure: decides the grid-template-columns string for a given viewport width. Extracted so the
// collapse decision is testable without mounting anything into a real DOM.
export function gridColumnsFor(viewportWidth: number, options: ClassicLayoutOptions, hasSidebar: boolean): string {
  if (!hasSidebar) return "1fr";
  const collapseBelow = options.collapseBelow ?? 768;
  if (viewportWidth < collapseBelow) return "1fr";
  const sidebarWidth = options.sidebarWidth ?? "240px";
  return `${sidebarWidth} 1fr`;
}

// Pure: decides whether the sidebar should render before or after main in the grid's visual
// order when stacked - sidebar on top, same as a typical nav-then-content page.
export function gridAreasFor(hasSidebar: boolean): string {
  return hasSidebar ? '"header header" "sidebar main" "footer footer"' : '"header" "main" "footer"';
}

export class ClassicLayout {
  private readonly containerEl: HTMLElement;
  private readonly slots: ClassicLayoutSlots;
  private readonly options: ClassicLayoutOptions;
  private resizeObserver: ResizeObserver | null = null;

  constructor(containerEl: HTMLElement, slots: ClassicLayoutSlots, options: ClassicLayoutOptions = {}) {
    this.containerEl = containerEl;
    this.slots = slots;
    this.options = options;
    ensureBaseStyle(containerEl.ownerDocument ?? document);

    containerEl.classList.add("fwa-classic-layout");
    containerEl.append(slots.header);
    if (slots.sidebar) containerEl.append(slots.sidebar);
    containerEl.append(slots.main);
    if (slots.footer) containerEl.append(slots.footer);

    slots.header.style.gridArea = "header";
    if (slots.sidebar) slots.sidebar.style.gridArea = "sidebar";
    slots.main.style.gridArea = "main";
    if (slots.footer) slots.footer.style.gridArea = "footer";

    this.relayout(containerEl.clientWidth || window.innerWidth);
  }

  // Re-applies the grid for a given viewport width - call this from a resize listener, or let
  // observeResize() do it automatically.
  relayout(viewportWidth: number): void {
    const hasSidebar = !!this.slots.sidebar;
    this.containerEl.style.gridTemplateColumns = gridColumnsFor(viewportWidth, this.options, hasSidebar);
    this.containerEl.style.gridTemplateAreas = gridAreasFor(hasSidebar);
  }

  observeResize(): () => void {
    this.resizeObserver = new ResizeObserver(([entry]) => {
      if (!entry) return;
      this.relayout(entry.contentRect.width);
    });
    this.resizeObserver.observe(this.containerEl);
    return () => this.resizeObserver?.disconnect();
  }
}

function ensureBaseStyle(doc: Document): void {
  if (doc.getElementById(STYLE_ID)) return;
  const style = doc.createElement("style");
  style.id = STYLE_ID;
  style.textContent = `
    .fwa-classic-layout {
      display: grid;
      grid-template-rows: auto 1fr auto;
      min-height: 100vh;
      gap: 0;
    }
  `;
  doc.head.append(style);
}
