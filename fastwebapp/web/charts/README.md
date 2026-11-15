# charts (TypeScript)

Bar and line charts on a `<canvas>`. The layout maths (`computeBarLayout`, `computeLineLayout`) is pure and tested
without a DOM; `drawBarChart` and `drawLineChart` only paint the result.

Depends on: nothing. Browser canvas only.

## Interface

    charts.ts   computeBarLayout, computeLineLayout, drawBarChart(canvas, data, color), drawLineChart(canvas, values, color)

## Test

    cd fastwebapp && npm test
