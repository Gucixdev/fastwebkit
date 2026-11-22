# export (TypeScript)

Tables out of and into files without a library: CSV and XLSX writers, a CSV parser, an XLSX reader, and the stored
(uncompressed) ZIP both formats need.

Depends on: nothing outside the folder. Inside it `export.ts` uses `export_zip.ts`, and `export_xlsx-read.ts` reads
what `export.ts` writes.

## Interface

    export.ts            toCSV, toXLSX, parseCSV, columnLetter, columnsFrom, downloadCSV, downloadXLSX, downloadBlob
    export_zip.ts        zipStore, unzipStore, crc32
    export_xlsx-read.ts  parseXLSX, readZipEntries (inflates deflate streams with the browser's DecompressionStream)

## Test

    cd fastwebapp && npm test
