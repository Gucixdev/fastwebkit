import { describe, expect, it } from "vitest";
import { toXLSX } from "./export";
import { parseXLSX, readZipEntries } from "./xlsx-read";

async function deflateRaw(data: Uint8Array): Promise<Uint8Array> {
  const stream = new ReadableStream<Uint8Array>({
    start(controller) {
      controller.enqueue(data);
      controller.close();
    },
  }).pipeThrough(new CompressionStream("deflate-raw"));
  return new Uint8Array(await new Response(stream).arrayBuffer());
}

function crc32(buf: Uint8Array): number {
  let crc = 0xffffffff;
  for (const byte of buf) {
    crc ^= byte;
    for (let j = 0; j < 8; j++) crc = (crc >>> 1) ^ (0xedb88320 & -(crc & 1));
  }
  return (crc ^ 0xffffffff) >>> 0;
}

class Writer {
  chunks: Uint8Array[] = [];
  length = 0;
  u16(v: number) {
    this.push(Uint8Array.of(v & 0xff, (v >>> 8) & 0xff));
  }
  u32(v: number) {
    this.push(Uint8Array.of(v & 0xff, (v >>> 8) & 0xff, (v >>> 16) & 0xff, (v >>> 24) & 0xff));
  }
  push(b: Uint8Array) {
    this.chunks.push(b);
    this.length += b.length;
  }
  bytes(): Uint8Array {
    const out = new Uint8Array(this.length);
    let off = 0;
    for (const c of this.chunks) {
      out.set(c, off);
      off += c.length;
    }
    return out;
  }
}

// Builds a single-entry zip with an explicit compression method, independent of this project's
// own store-only zip.ts writer - specifically so the deflate (method 8) path through
// readZipEntries has something real to decompress in a test, since toXLSX never produces one.
async function buildMinimalZip(name: string, data: Uint8Array, method: 0 | 8): Promise<Uint8Array> {
  const nameBytes = new TextEncoder().encode(name);
  const payload = method === 8 ? await deflateRaw(data) : data;
  const crc = crc32(data);

  const out = new Writer();
  const localOffset = 0;
  out.u32(0x04034b50);
  out.u16(20);
  out.u16(0);
  out.u16(method);
  out.u16(0);
  out.u16(0);
  out.u32(crc);
  out.u32(payload.length);
  out.u32(data.length);
  out.u16(nameBytes.length);
  out.u16(0);
  out.push(nameBytes);
  out.push(payload);

  const centralStart = out.length;
  out.u32(0x02014b50);
  out.u16(20);
  out.u16(20);
  out.u16(0);
  out.u16(method);
  out.u16(0);
  out.u16(0);
  out.u32(crc);
  out.u32(payload.length);
  out.u32(data.length);
  out.u16(nameBytes.length);
  out.u16(0);
  out.u16(0);
  out.u16(0);
  out.u16(0);
  out.u32(0);
  out.u32(localOffset);
  out.push(nameBytes);
  const centralSize = out.length - centralStart;

  out.u32(0x06054b50);
  out.u16(0);
  out.u16(0);
  out.u16(1);
  out.u16(1);
  out.u32(centralSize);
  out.u32(centralStart);
  out.u16(0);

  return out.bytes();
}

describe("readZipEntries", () => {
  it("reads back a stored (uncompressed) entry", async () => {
    const data = new TextEncoder().encode("hello stored world");
    const zip = await buildMinimalZip("hello.txt", data, 0);
    const entries = await readZipEntries(zip);
    expect(new TextDecoder().decode(entries.get("hello.txt"))).toBe("hello stored world");
  });

  it("inflates a deflate-compressed entry", async () => {
    const data = new TextEncoder().encode("hello deflated world, repeated repeated repeated repeated");
    const zip = await buildMinimalZip("hello.txt", data, 8);
    const entries = await readZipEntries(zip);
    expect(new TextDecoder().decode(entries.get("hello.txt"))).toBe(new TextDecoder().decode(data));
  });

  it("throws on bytes that aren't a zip at all", async () => {
    await expect(readZipEntries(new TextEncoder().encode("not a zip"))).rejects.toThrow();
  });
});

describe("parseXLSX", () => {
  it("round-trips what toXLSX just wrote", async () => {
    const rows = [
      { Name: "Alice", Age: 30 },
      { Name: "Bob", Age: 25 },
    ];
    const xlsx = toXLSX(rows);
    const parsed = await parseXLSX(xlsx);
    expect(parsed).toEqual([
      { Name: "Alice", Age: "30" },
      { Name: "Bob", Age: "25" },
    ]);
  });

  it("returns an empty array for a sheet with only a header row", async () => {
    const xlsx = toXLSX([], ["Name", "Age"]);
    const parsed = await parseXLSX(xlsx);
    expect(parsed).toEqual([]);
  });
});
