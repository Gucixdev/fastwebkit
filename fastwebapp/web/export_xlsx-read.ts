// Reading a .xlsx back, for previewing an uploaded spreadsheet rather than just writing one.
// Unlike export.ts's writer (which only ever produces "stored"/uncompressed zip entries), a real
// file from Excel/LibreOffice is near-certainly deflate-compressed - so this needs an actual zip
// *reader* (central directory + local headers, a plain byte format, no compression math) plus
// something to inflate deflate-compressed entries. That inflate step uses the browser/Node native
// DecompressionStream("deflate-raw") rather than a hand-rolled DEFLATE decoder: same call already
// made for Web Crypto/BarcodeDetector elsewhere in this project - a platform API beats hand-
// rolling something this easy to get subtly wrong. Cell/sheet XML is parsed with the native
// DOMParser for the same reason, rather than a regex walk over arbitrary third-party XML.

function readU16(view: DataView, off: number): number {
  return view.getUint16(off, true);
}
function readU32(view: DataView, off: number): number {
  return view.getUint32(off, true);
}

async function inflateRaw(data: Uint8Array): Promise<Uint8Array> {
  // A plain ReadableStream rather than Blob.stream() - jsdom's Blob polyfill (used by this
  // project's own test environment) doesn't implement .stream(), so going through Blob at all
  // would make this untestable; a hand-built single-chunk stream works the same everywhere.
  const stream = new ReadableStream<Uint8Array>({
    start(controller) {
      controller.enqueue(data);
      controller.close();
    },
  }).pipeThrough(new DecompressionStream("deflate-raw"));
  return new Uint8Array(await new Response(stream).arrayBuffer());
}

// Parses a zip's central directory + local headers to pull out every entry by name, inflating
// any deflate-compressed ones. Central directory sizes/offsets are trusted over local-header
// ones (the local header can carry streamed/zero sizes), only the local header's name+extra
// lengths are needed, to find where an entry's actual data starts.
export async function readZipEntries(bytes: Uint8Array): Promise<Map<string, Uint8Array>> {
  const view = new DataView(bytes.buffer, bytes.byteOffset, bytes.byteLength);
  const EOCD_SIG = 0x06054b50;

  let eocdOffset = -1;
  const scanBack = Math.min(bytes.length, 65557); // 22-byte EOCD + max 65535-byte comment
  for (let i = bytes.length - 22; i >= bytes.length - scanBack && i >= 0; i--) {
    if (readU32(view, i) === EOCD_SIG) {
      eocdOffset = i;
      break;
    }
  }
  if (eocdOffset < 0) throw new Error("readZipEntries: not a zip file (no end-of-central-directory record)");

  const totalEntries = readU16(view, eocdOffset + 10);
  let cdOffset = readU32(view, eocdOffset + 16);

  const decoder = new TextDecoder();
  const entries = new Map<string, Uint8Array>();

  for (let i = 0; i < totalEntries; i++) {
    if (readU32(view, cdOffset) !== 0x02014b50) {
      throw new Error("readZipEntries: malformed central directory entry");
    }
    const method = readU16(view, cdOffset + 10);
    const compressedSize = readU32(view, cdOffset + 20);
    const nameLen = readU16(view, cdOffset + 28);
    const extraLen = readU16(view, cdOffset + 30);
    const commentLen = readU16(view, cdOffset + 32);
    const localOffset = readU32(view, cdOffset + 42);
    const name = decoder.decode(bytes.subarray(cdOffset + 46, cdOffset + 46 + nameLen));

    const localNameLen = readU16(view, localOffset + 26);
    const localExtraLen = readU16(view, localOffset + 28);
    const dataStart = localOffset + 30 + localNameLen + localExtraLen;
    const compressed = bytes.subarray(dataStart, dataStart + compressedSize);

    entries.set(name, method === 0 ? compressed : await inflateRaw(compressed));
    cdOffset += 46 + nameLen + extraLen + commentLen;
  }
  return entries;
}

function parseSharedStrings(xml: string | undefined): string[] {
  if (!xml) return [];
  const doc = new DOMParser().parseFromString(xml, "application/xml");
  return [...doc.getElementsByTagName("si")].map((si) => si.textContent ?? "");
}

function cellValue(c: Element, sharedStrings: string[]): string {
  const type = c.getAttribute("t");
  if (type === "inlineStr") return c.getElementsByTagName("t")[0]?.textContent ?? "";
  const v = c.getElementsByTagName("v")[0]?.textContent ?? "";
  return type === "s" ? sharedStrings[Number(v)] ?? "" : v;
}

// "C5" -> 2 (0-indexed) - the inverse of export.ts's columnLetter, needed because a sheet's
// cells are sparse: an empty cell is often omitted entirely rather than written as "", so column
// position has to come from each <c>'s own r="..." ref, not from its position in the row.
function columnIndexFromRef(ref: string): number {
  const letters = ref.match(/^[A-Z]+/)?.[0] ?? "A";
  let n = 0;
  for (const ch of letters) n = n * 26 + (ch.charCodeAt(0) - 64);
  return n - 1;
}

// Reads the first sheet of a .xlsx into the same row-of-objects shape toCSV/toXLSX take, using
// row 1 as the header. Only the first sheet is read - this is a preview, not a full workbook
// import.
export async function parseXLSX(bytes: Uint8Array): Promise<Record<string, string>[]> {
  const entries = await readZipEntries(bytes);
  const sheet = entries.get("xl/worksheets/sheet1.xml");
  if (!sheet) throw new Error("parseXLSX: no xl/worksheets/sheet1.xml in this file");

  const decoder = new TextDecoder();
  const sharedStrings = parseSharedStrings(
    entries.has("xl/sharedStrings.xml") ? decoder.decode(entries.get("xl/sharedStrings.xml")!) : undefined,
  );

  const doc = new DOMParser().parseFromString(decoder.decode(sheet), "application/xml");
  const grid: string[][] = [...doc.getElementsByTagName("row")].map((rowEl) => {
    const row: string[] = [];
    for (const c of [...rowEl.getElementsByTagName("c")]) {
      const ref = c.getAttribute("r");
      row[ref ? columnIndexFromRef(ref) : row.length] = cellValue(c, sharedStrings);
    }
    for (let i = 0; i < row.length; i++) if (row[i] === undefined) row[i] = "";
    return row;
  });
  if (grid.length === 0) return [];

  const [header = [], ...dataRows] = grid;
  return dataRows.map((row) => {
    const obj: Record<string, string> = {};
    header.forEach((h, i) => (obj[h || `col${i + 1}`] = row[i] ?? ""));
    return obj;
  });
}
