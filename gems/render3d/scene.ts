// A WASM-canvas renderer was considered for the whole framework and rejected (loses
// accessibility/SEO/DOM form ecosystem that a Rails-like CRUD framework needs by default) - see
// project discussion. This gem is the opt-in version of that idea: a thin Three.js wrapper any
// one page can mount for a hero/showcase section, without it being the framework's foundation.
import * as THREE from "three";

export interface Scene3DOptions {
  background?: THREE.ColorRepresentation;
  cameraDistance?: number;
}

// Pure guard extracted out of resize(): a ResizeObserver can fire with a 0-size contentRect
// (element briefly detached, display:none, etc.) and feeding that straight to the renderer/
// camera would divide by zero on the aspect ratio. Exported so it's testable without a real
// WebGL context, which this environment doesn't have (see scene.test.ts).
export function isValidSize(width: number, height: number): boolean {
  return width > 0 && height > 0;
}

// Pure: caps devicePixelRatio at `max` - a 3x/4x phone display rendering every pixel at full
// resolution costs far more GPU time than the visual gain is worth.
export function clampPixelRatio(ratio: number, max = 2): number {
  return Math.min(ratio, max);
}

export class Scene3D {
  readonly scene = new THREE.Scene();
  readonly camera: THREE.PerspectiveCamera;
  readonly renderer: THREE.WebGLRenderer;
  private resizeObserver: ResizeObserver;
  private frame = 0;
  private updaters: Array<(dt: number) => void> = [];
  private lastTime = performance.now();

  constructor(private readonly container: HTMLElement, opts: Scene3DOptions = {}) {
    if (opts.background !== undefined) this.scene.background = new THREE.Color(opts.background);

    this.camera = new THREE.PerspectiveCamera(50, 1, 0.1, 100);
    this.camera.position.set(0, 0, opts.cameraDistance ?? 4);

    this.renderer = new THREE.WebGLRenderer({ antialias: true, alpha: opts.background === undefined });
    container.appendChild(this.renderer.domElement);

    this.scene.add(new THREE.AmbientLight(0xffffff, 0.6));
    const sun = new THREE.DirectionalLight(0xffffff, 1.2);
    sun.position.set(3, 4, 5);
    this.scene.add(sun);

    this.resizeObserver = new ResizeObserver(([entry]) => {
      if (!entry) return;
      const { width, height } = entry.contentRect;
      this.resize(width, height);
    });
    this.resizeObserver.observe(container);
    this.resize(container.clientWidth, container.clientHeight);

    this.loop = this.loop.bind(this);
    this.frame = requestAnimationFrame(this.loop);
  }

  // Register a per-frame callback (dt in seconds) - e.g. rotating a mesh. Kept generic rather
  // than baking "rotate" into the gem, so any page can drive its own motion.
  onFrame(fn: (dt: number) => void): () => void {
    this.updaters.push(fn);
    return () => {
      this.updaters = this.updaters.filter((f) => f !== fn);
    };
  }

  dispose(): void {
    cancelAnimationFrame(this.frame);
    this.resizeObserver.disconnect();
    this.renderer.dispose();
    this.container.removeChild(this.renderer.domElement);
  }

  private resize(width: number, height: number): void {
    if (!isValidSize(width, height)) return;
    this.camera.aspect = width / height;
    this.camera.updateProjectionMatrix();
    this.renderer.setSize(width, height);
    this.renderer.setPixelRatio(clampPixelRatio(window.devicePixelRatio));
  }

  private loop(now: number): void {
    const dt = (now - this.lastTime) / 1000;
    this.lastTime = now;
    for (const fn of this.updaters) fn(dt);
    this.renderer.render(this.scene, this.camera);
    this.frame = requestAnimationFrame(this.loop);
  }
}
