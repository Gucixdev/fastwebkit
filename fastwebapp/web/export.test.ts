import { describe, it, expect } from "vitest";
import { toCSV, columnLetter, columnsFrom, toXLSX, parseCSV } from "./export";
import { unzipStore } from "./export_zip";

describe("columnsFrom", () => {
  it("takes column names from the first row's keys", () => {
    expect(columnsFrom([{ a: 1, b: 2 }, { a: 3, b: 4 }])).toEqual(["a", "b"]);
  });
  it("is empty for no rows", () => {
    expect(columnsFrom([])).toEqual([]);
  });
});

describe("toCSV", () => {
  it("writes a header row then one row per item, CRLF-terminated", () => {
    const csv = toCSV([{ title: "Pi Case", price: 3499 }]);
    expect(csv).toBe("title,price\r\nPi Case,3499\r\n");
  });

  it("quotes fields containing commas, quotes, or newlines", () => {
    const csv = toCSV([{ note: 'has, a comma' }, { note: 'has "quotes"' }, { note: "has\nnewline" }]);
    const lines = csv.split("\r\n");
    expect(lines[1]).toBe('"has, a comma"');
    expect(lines[2]).toBe('"has ""quotes"""');
    expect(lines[3]).toBe('"has\nnewline"');
  });

  it("renders null/undefined as an empty field", () => {
    expect(toCSV([{ a: null, b: undefined }])).toBe("a,b\r\n,\r\n");
  });

  it("honors an explicit column order/subset", () => {
    expect(toCSV([{ a: 1, b: 2, c: 3 }], ["c", "a"])).toBe("c,a\r\n3,1\r\n");
  });
});

describe("columnLetter", () => {
  it("covers single letters A-Z", () => {
    expect(columnLetter(0)).toBe("A");
    expect(columnLetter(25)).toBe("Z");
  });
  it("rolls over into double letters", () => {
    expect(columnLetter(26)).toBe("AA");
    expect(columnLetter(27)).toBe("AB");
  });
});

describe("toXLSX", () => {
  it("produces a zip whose sheet1.xml contains the header and row values", () => {
    const bytes = toXLSX([{ title: "Pi Case", price_cents: 3499 }]);
    const entries = unzipStore(bytes);
    const names = entries.map((e) => e.name);

    expect(names).toContain("[Content_Types].xml");
    expect(names).toContain("xl/workbook.xml");
    expect(names).toContain("xl/worksheets/sheet1.xml");

    const sheet = new TextDecoder().decode(entries.find((e) => e.name === "xl/worksheets/sheet1.xml")!.data);
    expect(sheet).toContain("<t xml:space=\"preserve\">title</t>");
    expect(sheet).toContain("<t xml:space=\"preserve\">Pi Case</t>");
    expect(sheet).toContain("<v>3499</v>"); // numeric cell, not quoted as a string
  });

  it("escapes XML-significant characters in string cells", () => {
    const bytes = toXLSX([{ note: "a < b & c > d" }]);
    const sheet = new TextDecoder().decode(
      unzipStore(bytes).find((e) => e.name === "xl/worksheets/sheet1.xml")!.data,
    );
    expect(sheet).toContain("a &lt; b &amp; c &gt; d");
    expect(sheet).not.toContain("a < b");
  });
});

describe("parseCSV", () => {
  it("round-trips what toCSV just wrote", () => {
    const csv = toCSV([
      { title: "Pi Case", price: 3499 },
      { title: "Fan", price: 999 },
    ]);
    expect(parseCSV(csv)).toEqual([
      { title: "Pi Case", price: "3499" },
      { title: "Fan", price: "999" },
    ]);
  });

  it("handles quoted fields with embedded commas, quotes, and newlines", () => {
    const csv = toCSV([{ note: 'a, "b"\nc', title: "x" }]);
    expect(parseCSV(csv)).toEqual([{ note: 'a, "b"\nc', title: "x" }]);
  });

  it("is empty for header-only input", () => {
    expect(parseCSV("title,price\r\n")).toEqual([]);
  });

  it("is empty for an empty string", () => {
    expect(parseCSV("")).toEqual([]);
  });
});
