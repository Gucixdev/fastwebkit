# table (TypeScript)

A sortable, paged data table. `datatable.ts` renders; `pagination.ts` is the page maths and widget it uses.

Depends on: nothing outside the folder (`datatable.ts` imports `pagination.ts`).

## Interface

    datatable.ts    DataTable, sortRows, Column, SortDirection
    pagination.ts   Pagination, pageCount, clampPage, paginate

## Test

    cd fastwebapp && npm test
