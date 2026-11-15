// The generic CRUD list view: sortable columns, client-side pagination (via the pagination
// gem), an optional search query (left to the caller - datatable just takes whatever row array
// it's given, searchbar/searchbar.ts decides what's in it). Sort/filter are pure functions so
// the actual ordering logic is tested without touching the DOM; render() is a thin consumer.
import { Pagination } from "./pagination";

export interface Column<T> {
  key: string;
  label: string;
  sortable?: boolean;
  render?: (row: T) => string;
  value?: (row: T) => string | number;
}

export type SortDirection = "asc" | "desc";

// Pure: stable sort by a column's `value()` (falls back to `row[key]`), ascending or descending.
export function sortRows<T extends Record<string, unknown>>(
  rows: T[],
  column: Column<T>,
  direction: SortDirection,
): T[] {
  const getValue = column.value ?? ((row: T) => row[column.key] as string | number);
  const withIndex = rows.map((row, i) => ({ row, i }));
  withIndex.sort((a, b) => {
    const av = getValue(a.row);
    const bv = getValue(b.row);
    let cmp = 0;
    if (av < bv) cmp = -1;
    else if (av > bv) cmp = 1;
    if (cmp === 0) return a.i - b.i; // stable: preserve original order on ties
    return direction === "asc" ? cmp : -cmp;
  });
  return withIndex.map((w) => w.row);
}

export interface DataTableOptions<T> {
  columns: Column<T>[];
  pageSize?: number;
}

export class DataTable<T extends Record<string, unknown>> {
  readonly el: HTMLElement;
  private readonly table: HTMLTableElement;
  private readonly tbody: HTMLTableSectionElement;
  private readonly pagination: Pagination;
  private readonly options: DataTableOptions<T>;
  private rows: T[] = [];
  private sortKey: string | null = null;
  private sortDirection: SortDirection = "asc";

  constructor(container: HTMLElement, options: DataTableOptions<T>) {
    this.options = options;
    this.el = document.createElement("div");
    this.el.className = "fwa-datatable";

    this.table = document.createElement("table");
    const thead = document.createElement("thead");
    const headRow = document.createElement("tr");
    for (const col of options.columns) {
      const th = document.createElement("th");
      th.textContent = col.label;
      if (col.sortable) {
        th.classList.add("fwa-datatable-sortable");
        th.addEventListener("click", () => this.toggleSort(col.key));
      }
      headRow.append(th);
    }
    thead.append(headRow);
    this.tbody = document.createElement("tbody");
    this.table.append(thead, this.tbody);
    this.el.append(this.table);

    const paginationEl = document.createElement("div");
    this.el.append(paginationEl);
    this.pagination = new Pagination(paginationEl);
    this.pagination.onChange(() => this.renderBody());

    container.append(this.el);
  }

  setRows(rows: T[]): void {
    this.rows = rows;
    this.pagination.update(this.rows.length, this.options.pageSize ?? 10);
    this.renderBody();
  }

  private toggleSort(key: string): void {
    if (this.sortKey === key) {
      this.sortDirection = this.sortDirection === "asc" ? "desc" : "asc";
    } else {
      this.sortKey = key;
      this.sortDirection = "asc";
    }
    this.renderBody();
  }

  private sortedRows(): T[] {
    if (!this.sortKey) return this.rows;
    const column = this.options.columns.find((c) => c.key === this.sortKey);
    if (!column) return this.rows;
    return sortRows(this.rows, column, this.sortDirection);
  }

  private renderBody(): void {
    const pageSize = this.options.pageSize ?? 10;
    const sorted = this.sortedRows();
    this.pagination.update(sorted.length, pageSize);
    const start = (this.pagination.currentPage - 1) * pageSize;
    const pageRows = sorted.slice(start, start + pageSize);

    this.tbody.innerHTML = "";
    for (const row of pageRows) {
      const tr = document.createElement("tr");
      for (const col of this.options.columns) {
        const td = document.createElement("td");
        td.textContent = col.render ? col.render(row) : String(row[col.key] ?? "");
        tr.append(td);
      }
      this.tbody.append(tr);
    }
  }
}
