// Table export to CSV and to a real .xlsx (opens in LibreOffice Calc, Excel, or anything else
// OOXML - no OnlyOffice/cloud-suite integration, just a file the browser downloads). CSV is
// plain text, trivial to hand-write; .xlsx is a zip of small XML parts (see zip.ts for why that
// didn't need a dependency either) - using inline strings (t="inlineStr") instead of a shared-
// strings table keeps the XML side just as simple as the zip side.
import { zipStore } from "./export_zip";

export function columnsFrom<T extends Record<string, unknown>>(rows: T[]): string[] {
  return rows.length === 0 ? [] : Object.keys(rows[0]!);
}

function csvField(value: unknown): string {
  const s = value === null || value === undefined ? "" : String(value);
  if (/[",\n]/.test(s)) return `"${s.replace(/"/g, '""')}"`;
  return s;
}

export function toCSV<T extends Record<string, unknown>>(rows: T[], columns?: string[]): string {
  const cols = columns ?? columnsFrom(rows);
  const lines = [cols.map(csvField).join(",")];
  for (const row of rows) {
    lines.push(cols.map((c) => csvField(row[c])).join(","));
  }
  return lines.join("\r\n") + "\r\n";
}

function xmlEscape(s: string): string {
  return s.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
}

// A-Z, then AA, AB, ... - standard spreadsheet column naming, 1-indexed.
export function columnLetter(index: number): string {
  let n = index + 1;
  let letters = "";
  while (n > 0) {
    const rem = (n - 1) % 26;
    letters = String.fromCharCode(65 + rem) + letters;
    n = Math.floor((n - 1) / 26);
  }
  return letters;
}

function cellXML(colIndex: number, rowIndex: number, value: unknown): string {
  const ref = `${columnLetter(colIndex)}${rowIndex}`;
  if (typeof value === "number" && Number.isFinite(value)) {
    return `<c r="${ref}"><v>${value}</v></c>`;
  }
  const text = value === null || value === undefined ? "" : String(value);
  return `<c r="${ref}" t="inlineStr"><is><t xml:space="preserve">${xmlEscape(text)}</t></is></c>`;
}

function sheetXML<T extends Record<string, unknown>>(rows: T[], columns: string[]): string {
  const headerRow = `<row r="1">${columns.map((c, i) => cellXML(i, 1, c)).join("")}</row>`;
  const dataRows = rows.map(
    (row, r) => `<row r="${r + 2}">${columns.map((c, i) => cellXML(i, r + 2, row[c])).join("")}</row>`,
  );
  return (
    `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
    `<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">` +
    `<sheetData>${headerRow}${dataRows.join("")}</sheetData>` +
    `</worksheet>`
  );
}

const CONTENT_TYPES = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
<Default Extension="xml" ContentType="application/xml"/>
<Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/>
<Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>
</Types>`;

const ROOT_RELS = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/>
</Relationships>`;

const WORKBOOK_XML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">
<sheets><sheet name="Sheet1" sheetId="1" r:id="rId1"/></sheets>
</workbook>`;

const WORKBOOK_RELS = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/>
</Relationships>`;

export function toXLSX<T extends Record<string, unknown>>(rows: T[], columns?: string[]): Uint8Array<ArrayBuffer> {
  const cols = columns ?? columnsFrom(rows);
  const enc = new TextEncoder();
  return zipStore([
    { name: "[Content_Types].xml", data: enc.encode(CONTENT_TYPES) },
    { name: "_rels/.rels", data: enc.encode(ROOT_RELS) },
    { name: "xl/workbook.xml", data: enc.encode(WORKBOOK_XML) },
    { name: "xl/_rels/workbook.xml.rels", data: enc.encode(WORKBOOK_RELS) },
    { name: "xl/worksheets/sheet1.xml", data: enc.encode(sheetXML(rows, cols)) },
  ]);
}

// Browser-only; not exercised by unit tests (no real download sink in jsdom), kept thin on
// purpose so the parts worth testing (toCSV/toXLSX above) don't depend on DOM download plumbing.
export function downloadBlob(filename: string, data: string | Uint8Array<ArrayBuffer>, mime: string): void {
  const blob = new Blob([data], { type: mime });
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = filename;
  a.click();
  URL.revokeObjectURL(url);
}

export function downloadCSV<T extends Record<string, unknown>>(filename: string, rows: T[], columns?: string[]): void {
  downloadBlob(filename, toCSV(rows, columns), "text/csv;charset=utf-8");
}

export function downloadXLSX<T extends Record<string, unknown>>(filename: string, rows: T[], columns?: string[]): void {
  downloadBlob(filename, toXLSX(rows, columns), "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet");
}

// parseCSV is toCSV's inverse, for previewing an uploaded .csv rather than just exporting one.
// A state machine rather than a split-on-comma regex, since a quoted field can itself contain
// commas, newlines and escaped ("") quotes - exactly what csvField above writes.
function parseCSVRows(text: string): string[][] {
  const rows: string[][] = [];
  let row: string[] = [];
  let field = "";
  let inQuotes = false;
  let i = 0;
  while (i < text.length) {
    const ch = text[i];
    if (inQuotes) {
      if (ch === '"') {
        if (text[i + 1] === '"') {
          field += '"';
          i += 2;
          continue;
        }
        inQuotes = false;
        i++;
        continue;
      }
      field += ch;
      i++;
      continue;
    }
    if (ch === '"') {
      inQuotes = true;
      i++;
      continue;
    }
    if (ch === ",") {
      row.push(field);
      field = "";
      i++;
      continue;
    }
    if (ch === "\r") {
      i++;
      continue;
    }
    if (ch === "\n") {
      row.push(field);
      rows.push(row);
      row = [];
      field = "";
      i++;
      continue;
    }
    field += ch;
    i++;
  }
  if (field !== "" || row.length > 0) {
    row.push(field);
    rows.push(row);
  }
  return rows;
}

export function parseCSV(text: string): Record<string, string>[] {
  const rows = parseCSVRows(text);
  if (rows.length === 0) return [];
  const [header = [], ...dataRows] = rows;
  return dataRows.map((row) => {
    const obj: Record<string, string> = {};
    header.forEach((h, i) => (obj[h || `col${i + 1}`] = row[i] ?? ""));
    return obj;
  });
}
