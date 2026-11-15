// Pure paging math (slice a page out of a list, count pages, clamp an out-of-range page number)
// plus a thin "< 1 2 3 >" control - used standalone or by datatable.ts.
export function pageCount(totalItems: number, pageSize: number): number {
  if (pageSize <= 0) return 0;
  return Math.max(1, Math.ceil(totalItems / pageSize));
}

export function clampPage(page: number, totalPages: number): number {
  return Math.min(Math.max(1, page), Math.max(1, totalPages));
}

export function paginate<T>(items: T[], page: number, pageSize: number): T[] {
  const total = pageCount(items.length, pageSize);
  const safePage = clampPage(page, total);
  const start = (safePage - 1) * pageSize;
  return items.slice(start, start + pageSize);
}

export class Pagination {
  readonly el: HTMLElement;
  private page = 1;
  private totalPages = 1;
  private readonly listeners = new Set<(page: number) => void>();

  constructor(container: HTMLElement) {
    this.el = document.createElement("div");
    this.el.className = "fwa-pagination";
    container.append(this.el);
  }

  // Call whenever the underlying item count or page size changes.
  update(totalItems: number, pageSize: number): void {
    this.totalPages = pageCount(totalItems, pageSize);
    this.page = clampPage(this.page, this.totalPages);
    this.render();
  }

  get currentPage(): number {
    return this.page;
  }

  onChange(fn: (page: number) => void): () => void {
    this.listeners.add(fn);
    return () => this.listeners.delete(fn);
  }

  private goto(page: number): void {
    const next = clampPage(page, this.totalPages);
    if (next === this.page) return;
    this.page = next;
    this.render();
    for (const fn of this.listeners) fn(this.page);
  }

  private render(): void {
    this.el.innerHTML = "";
    const prev = this.button("‹", () => this.goto(this.page - 1));
    prev.disabled = this.page <= 1;
    this.el.append(prev);

    for (let p = 1; p <= this.totalPages; p++) {
      const btn = this.button(String(p), () => this.goto(p));
      btn.classList.toggle("fwa-pagination-active", p === this.page);
      this.el.append(btn);
    }

    const next = this.button("›", () => this.goto(this.page + 1));
    next.disabled = this.page >= this.totalPages;
    this.el.append(next);
  }

  private button(label: string, onClick: () => void): HTMLButtonElement {
    const btn = document.createElement("button");
    btn.type = "button";
    btn.textContent = label;
    btn.addEventListener("click", onClick);
    return btn;
  }
}
