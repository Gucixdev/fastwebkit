// A from-scratch, "stored" (uncompressed) ZIP writer - no deflate, no dependency. The ZIP spec
// explicitly allows compression method 0 (store the bytes as-is), which only needs a CRC32 to
// be valid, not a compression algorithm - that's the whole reason this is hand-rollable at all.
// Files end up a bit larger than a real zip tool would produce; for the small exported tables
// this gem is for, that trade is worth not pulling in a zip/deflate dependency. Also exposes a
// matching reader, used by this gem's own tests to prove the writer output round-trips.
export interface ZipEntry {
  name: string;
  data: Uint8Array;
}

const LOCAL_SIG = 0x04034b50;
const CENTRAL_SIG = 0x02014b50;
const END_SIG = 0x06054b50;
const DOS_DATE = 0x0021; // 1980-01-01 - stored files don't need a real timestamp
const DOS_TIME = 0x0000;

export function crc32(buf: Uint8Array): number {
  let crc = 0xffffffff;
  for (let i = 0; i < buf.length; i++) {
    crc ^= buf[i]!;
    for (let j = 0; j < 8; j++) {
      crc = (crc >>> 1) ^ (0xedb88320 & -(crc & 1));
    }
  }
  return (crc ^ 0xffffffff) >>> 0;
}

class ByteWriter {
  private chunks: Uint8Array[] = [];
  length = 0;

  u16(v: number): this {
    return this.bytes(Uint8Array.of(v & 0xff, (v >>> 8) & 0xff));
  }
  u32(v: number): this {
    return this.bytes(Uint8Array.of(v & 0xff, (v >>> 8) & 0xff, (v >>> 16) & 0xff, (v >>> 24) & 0xff));
  }
  bytes(b: Uint8Array): this {
    this.chunks.push(b);
    this.length += b.length;
    return this;
  }
  utf8(s: string): this {
    return this.bytes(new TextEncoder().encode(s));
  }
  toUint8Array(): Uint8Array<ArrayBuffer> {
    const out = new Uint8Array(this.length);
    let offset = 0;
    for (const c of this.chunks) {
      out.set(c, offset);
      offset += c.length;
    }
    return out;
  }
}

export function zipStore(entries: ZipEntry[]): Uint8Array<ArrayBuffer> {
  const out = new ByteWriter();
  const centralOffsets: number[] = [];

  for (const entry of entries) {
    const nameBytes = new TextEncoder().encode(entry.name);
    const crc = crc32(entry.data);
    centralOffsets.push(out.length);

    out
      .u32(LOCAL_SIG)
      .u16(20) // version needed
      .u16(0) // flags
      .u16(0) // compression method: stored
      .u16(DOS_TIME)
      .u16(DOS_DATE)
      .u32(crc)
      .u32(entry.data.length) // compressed size == uncompressed for stored
      .u32(entry.data.length)
      .u16(nameBytes.length)
      .u16(0) // extra field length
      .bytes(nameBytes)
      .bytes(entry.data);
  }

  const centralStart = out.length;
  for (let i = 0; i < entries.length; i++) {
    const entry = entries[i]!;
    const nameBytes = new TextEncoder().encode(entry.name);
    const crc = crc32(entry.data);

    out
      .u32(CENTRAL_SIG)
      .u16(20) // version made by
      .u16(20) // version needed
      .u16(0) // flags
      .u16(0) // compression method
      .u16(DOS_TIME)
      .u16(DOS_DATE)
      .u32(crc)
      .u32(entry.data.length)
      .u32(entry.data.length)
      .u16(nameBytes.length)
      .u16(0) // extra field length
      .u16(0) // comment length
      .u16(0) // disk number
      .u16(0) // internal attrs
      .u32(0) // external attrs
      .u32(centralOffsets[i]!)
      .bytes(nameBytes);
  }
  const centralSize = out.length - centralStart;

  out
    .u32(END_SIG)
    .u16(0) // disk number
    .u16(0) // disk with central dir
    .u16(entries.length)
    .u16(entries.length)
    .u32(centralSize)
    .u32(centralStart)
    .u16(0); // comment length

  return out.toUint8Array();
}

class ByteReader {
  pos = 0;
  constructor(private readonly buf: Uint8Array) {}
  u16(): number {
    const v = this.buf[this.pos]! | (this.buf[this.pos + 1]! << 8);
    this.pos += 2;
    return v;
  }
  u32(): number {
    const v =
      (this.buf[this.pos]! |
        (this.buf[this.pos + 1]! << 8) |
        (this.buf[this.pos + 2]! << 16) |
        (this.buf[this.pos + 3]! << 24)) >>>
      0;
    this.pos += 4;
    return v;
  }
  bytes(n: number): Uint8Array {
    const v = this.buf.slice(this.pos, this.pos + n);
    this.pos += n;
    return v;
  }
}

// Reads back entries written by zipStore (stored method only). Used by this gem's own tests
// to prove the writer's output is a structurally valid zip, not just "looks like one".
export function unzipStore(data: Uint8Array): ZipEntry[] {
  const entries: ZipEntry[] = [];
  const r = new ByteReader(data);
  while (r.pos < data.length) {
    const sig = r.u32();
    if (sig !== LOCAL_SIG) break;
    r.u16(); // version needed
    r.u16(); // flags
    const method = r.u16();
    r.u16(); // mod time
    r.u16(); // mod date
    r.u32(); // crc32
    const compressedSize = r.u32();
    const uncompressedSize = r.u32();
    const nameLen = r.u16();
    const extraLen = r.u16();
    const name = new TextDecoder().decode(r.bytes(nameLen));
    r.bytes(extraLen);
    if (method !== 0) throw new Error(`unzipStore: entry ${name} uses unsupported compression method ${method}`);
    const fileData = r.bytes(compressedSize);
    if (fileData.length !== uncompressedSize) throw new Error(`unzipStore: size mismatch for ${name}`);
    entries.push({ name, data: fileData });
  }
  return entries;
}
