import { describe, it, expect } from "vitest";
import { sortRows, DataTable, type Column } from "./datatable";

interface Row {
  name: string;
  price: number;
  [key: string]: unknown;
}

describe("sortRows", () => {
  const column: Column<Row> = { key: "price", label: "Price" };

  it("sorts ascending by the column's value", () => {
    const rows: Row[] = [{ name: "b", price: 20 }, { name: "a", price: 10 }];
    expect(sortRows(rows, column, "asc").map((r) => r.name)).toEqual(["a", "b"]);
  });

  it("sorts descending", () => {
    const rows: Row[] = [{ name: "a", price: 10 }, { name: "b", price: 20 }];
    expect(sortRows(rows, column, "desc").map((r) => r.name)).toEqual(["b", "a"]);
  });

  it("is stable on ties - preserves original order among equal values", () => {
    const rows: Row[] = [
      { name: "first", price: 10 },
      { name: "second", price: 10 },
      { name: "third", price: 5 },
    ];
    expect(sortRows(rows, column, "asc").map((r) => r.name)).toEqual(["third", "first", "second"]);
  });

  it("uses a custom value() instead of the raw field when given", () => {
    const col: Column<Row> = { key: "name", label: "Name", value: (r) => r.name.length };
    const rows: Row[] = [{ name: "ccc", price: 0 }, { name: "a", price: 0 }, { name: "bb", price: 0 }];
    expect(sortRows(rows, col, "asc").map((r) => r.name)).toEqual(["a", "bb", "ccc"]);
  });
});

describe("DataTable", () => {
  const columns: Column<Row>[] = [
    { key: "name", label: "Name", sortable: true },
    { key: "price", label: "Price", sortable: true },
  ];

  it("renders a header cell per column and a row per item (within one page)", () => {
    const table = new DataTable<Row>(document.createElement("div"), { columns, pageSize: 10 });
    table.setRows([{ name: "Pi Case", price: 3499 }, { name: "Cable", price: 999 }]);

    const headers = Array.from(table.el.querySelectorAll("th")).map((th) => th.textContent);
    expect(headers).toEqual(["Name", "Price"]);

    const rows = table.el.querySelectorAll("tbody tr");
    expect(rows).toHaveLength(2);
  });

  it("clicking a sortable header sorts the body by that column", () => {
    const table = new DataTable<Row>(document.createElement("div"), { columns, pageSize: 10 });
    table.setRows([{ name: "b", price: 20 }, { name: "a", price: 10 }]);

    const nameHeader = Array.from(table.el.querySelectorAll("th")).find((th) => th.textContent === "Name")!;
    nameHeader.dispatchEvent(new Event("click"));

    const firstCell = table.el.querySelector("tbody tr td")!;
    expect(firstCell.textContent).toBe("a");
  });

  it("clicking the same header twice reverses the sort direction", () => {
    const table = new DataTable<Row>(document.createElement("div"), { columns, pageSize: 10 });
    table.setRows([{ name: "a", price: 10 }, { name: "b", price: 20 }]);
    const nameHeader = Array.from(table.el.querySelectorAll("th")).find((th) => th.textContent === "Name")!;

    nameHeader.dispatchEvent(new Event("click")); // asc: a, b
    nameHeader.dispatchEvent(new Event("click")); // desc: b, a

    const firstCell = table.el.querySelector("tbody tr td")!;
    expect(firstCell.textContent).toBe("b");
  });

  it("paginates rows beyond pageSize", () => {
    const table = new DataTable<Row>(document.createElement("div"), { columns, pageSize: 1 });
    table.setRows([{ name: "a", price: 1 }, { name: "b", price: 2 }, { name: "c", price: 3 }]);

    expect(table.el.querySelectorAll("tbody tr")).toHaveLength(1);
    expect(table.el.querySelectorAll(".fwa-pagination button")).toHaveLength(5); // prev,1,2,3,next
  });

  it("uses a column's custom render() for the cell text", () => {
    const priceColumns: Column<Row>[] = [
      { key: "name", label: "Name" },
      { key: "price", label: "Price", render: (r) => `$${(r.price / 100).toFixed(2)}` },
    ];
    const table = new DataTable<Row>(document.createElement("div"), { columns: priceColumns });
    table.setRows([{ name: "Pi Case", price: 3499 }]);

    const cells = Array.from(table.el.querySelectorAll("tbody td")).map((td) => td.textContent);
    expect(cells).toEqual(["Pi Case", "$34.99"]);
  });
});
