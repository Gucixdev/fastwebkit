// Spring-physics animation: a damped harmonic oscillator driving a number toward a target.
// No tweening curves to pick - stiffness/damping fully describe the motion, and a spring mid-
// flight that gets a new target decelerates smoothly from its current velocity instead of
// snapping, which is what makes spring-driven UI feel physical instead of canned.
export interface SpringConfig {
  stiffness?: number; // pull toward target; higher = faster, snappier
  damping?: number; // resistance; higher = less overshoot
  mass?: number;
  precision?: number; // considered "settled" once |target - x| and |v| are both below this
}

export class Spring {
  private x: number;
  private v = 0;
  private target: number;
  private readonly stiffness: number;
  private readonly damping: number;
  private readonly mass: number;
  private readonly precision: number;
  private frame = 0;
  private listeners = new Set<(x: number) => void>();

  constructor(initial: number, config: SpringConfig = {}) {
    this.x = initial;
    this.target = initial;
    this.stiffness = config.stiffness ?? 170;
    this.damping = config.damping ?? 26;
    this.mass = config.mass ?? 1;
    this.precision = config.precision ?? 0.01;
  }

  set(target: number): void {
    this.target = target;
    this.ensureRunning();
  }

  get value(): number {
    return this.x;
  }

  subscribe(fn: (x: number) => void): () => void {
    this.listeners.add(fn);
    return () => this.listeners.delete(fn);
  }

  private ensureRunning(): void {
    if (this.frame) return;
    let last = performance.now();
    const tick = (now: number) => {
      const dt = Math.min((now - last) / 1000, 1 / 30); // clamp so a dropped frame can't lurch
      last = now;

      const springForce = (this.target - this.x) * this.stiffness;
      const dampingForce = -this.v * this.damping;
      const acceleration = (springForce + dampingForce) / this.mass;
      this.v += acceleration * dt;
      this.x += this.v * dt;

      for (const fn of this.listeners) fn(this.x);

      const settled = Math.abs(this.target - this.x) < this.precision && Math.abs(this.v) < this.precision;
      if (settled) {
        this.x = this.target;
        this.v = 0;
        this.frame = 0;
        for (const fn of this.listeners) fn(this.x);
        return;
      }
      this.frame = requestAnimationFrame(tick);
    };
    this.frame = requestAnimationFrame(tick);
  }
}
