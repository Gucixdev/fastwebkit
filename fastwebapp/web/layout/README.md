# layout (TypeScript)

Two ways to lay out a page. `ClassicLayout` is header, optional sidebar, main, footer on CSS grid, with the
column widths computed from the viewport. `ConstraintLayout` solves boxes with the Cassowary algorithm (kiwi.js).

Depends on: `kiwi.js` (npm) for the constraint layout.

## Interface

    layout_classic-layout.ts     ClassicLayout, gridColumnsFor, gridAreasFor
    layout_constraint-layout.ts  Box, ConstraintLayout, Operator, Strength

## Test

    cd fastwebapp && npm test
