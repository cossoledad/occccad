/** A wheel burst advances one image; inertial tails must accumulate a new step. */
export class ShowcaseWheel {
  private distance = 0;
  private lastInput = -Infinity;
  private lastStep = -Infinity;
  step(delta: number, mode: number, pageHeight: number, now: number): number {
    const pixels = delta * (mode === 1 ? 16 : mode === 2 ? pageHeight : 1);
    if (!Number.isFinite(pixels) || pixels === 0) return 0;
    if (now - this.lastInput > 180 || Math.sign(pixels) !== Math.sign(this.distance)) this.distance = 0;
    this.lastInput = now;
    if (now - this.lastStep < 700) { this.distance = 0; return 0; }
    this.distance += pixels;
    if (Math.abs(this.distance) < 48) return 0;
    const direction = Math.sign(this.distance);
    this.distance = 0;
    this.lastStep = now;
    return direction;
  }
}
