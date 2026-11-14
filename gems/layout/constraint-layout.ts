// Cassowary constraint layout, via kiwi.js (the same solver family behind iOS/macOS Auto
// Layout). The win over plain CSS isn't grids - flexbox already does those - it's relational
// constraints CSS can't express at all: "sidebar is always exactly 0.3x of main, regardless of
// viewport" holds on every screen size without a breakpoint list, because it's solved, not
// guessed per breakpoint. That's what "the layout looks the same everywhere" means here: one
// set of relationships, not one CSS rule per device class.
import * as kiwi from "kiwi.js";

export class Box {
  readonly left = new kiwi.Variable();
  readonly top = new kiwi.Variable();
  readonly width = new kiwi.Variable();
  readonly height = new kiwi.Variable();

  constructor(readonly el: HTMLElement) {
    el.style.position = "absolute";
  }

  apply(): void {
    this.el.style.left = `${this.left.value()}px`;
    this.el.style.top = `${this.top.value()}px`;
    this.el.style.width = `${this.width.value()}px`;
    this.el.style.height = `${this.height.value()}px`;
  }
}

// The container's own width/height, read from its real rendered size (via ResizeObserver) and
// fed into the solver as edit variables. Deliberately NOT a Box: a Box forces position:absolute
// on its element, which strips it out of normal flow and collapses it to shrink-to-fit size -
// backwards from "fill the viewport". This was the cause of the broken-looking first render:
// the container itself was wrapped in a Box, so it lost its CSS height:100vh/full width the
// instant the layout ran. The container keeps whatever CSS sizing it already has; only its
// children get positioned by the solver.
class ContainerSize {
  readonly width = new kiwi.Variable();
  readonly height = new kiwi.Variable();
}

export class ConstraintLayout {
  readonly solver = new kiwi.Solver();
  readonly container = new ContainerSize();
  private readonly boxes: Box[] = [];
  private resizeObserver: ResizeObserver | null = null;

  constructor(private readonly containerEl: HTMLElement) {
    containerEl.style.position = "relative";
    // the container's own width/height are edit variables: driven by actual layout (the
    // ResizeObserver below), not solved from other constraints.
    this.solver.addEditVariable(this.container.width, kiwi.Strength.strong);
    this.solver.addEditVariable(this.container.height, kiwi.Strength.strong);
  }

  addBox(el: HTMLElement): Box {
    const box = new Box(el);
    this.boxes.push(box);
    this.containerEl.appendChild(el);
    return box;
  }

  constrain(
    lhs: kiwi.Expression | kiwi.Variable,
    op: kiwi.Operator,
    rhs: kiwi.Expression | kiwi.Variable | number,
    strength: number = kiwi.Strength.required,
  ): kiwi.Constraint {
    // createConstraint already adds it to the solver - calling addConstraint again on the
    // result throws "duplicate constraint".
    return this.solver.createConstraint(lhs, op, rhs, strength);
  }

  solve(): void {
    this.solver.updateVariables();
    for (const box of this.boxes) box.apply();
  }

  // Re-solves on every real resize (not just window resize - any ancestor layout change), so
  // the constraints hold whether the container is the whole viewport or a card inside a grid.
  observeResize(): () => void {
    this.resizeObserver = new ResizeObserver(([entry]) => {
      if (!entry) return;
      const { width, height } = entry.contentRect;
      this.solver.suggestValue(this.container.width, width);
      this.solver.suggestValue(this.container.height, height);
      this.solve();
    });
    this.resizeObserver.observe(this.containerEl);
    return () => this.resizeObserver?.disconnect();
  }
}

export { Operator, Strength } from "kiwi.js";
