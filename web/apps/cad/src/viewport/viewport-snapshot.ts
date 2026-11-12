type Request = { resolve: (blob: Blob) => void; reject: (error: Error) => void };

// Only graphics belong in the image. Controls and command panels stay in the UI.
const svgStyles = ["color", "display", "visibility", "opacity", "fill", "fill-opacity", "stroke", "stroke-opacity",
  "stroke-width", "stroke-dasharray", "stroke-linecap", "stroke-linejoin", "font-family", "font-size", "font-weight",
  "font-variant-numeric", "text-anchor", "dominant-baseline", "paint-order", "vector-effect"];

function snapshotSVG(source: SVGSVGElement, width: number, height: number): string {
  const copy = source.cloneNode(true) as SVGSVGElement;
  const originals = [source, ...source.querySelectorAll<SVGElement>("*")];
  const copies = [copy, ...copy.querySelectorAll<SVGElement>("*")];
  originals.forEach((element, index) => {
    const style = getComputedStyle(element);
    for (const property of svgStyles) copies[index].style.setProperty(property, style.getPropertyValue(property));
    // The SVG viewBox maps CSS pixels to native pixels; strokes must follow DPR.
    if (style.getPropertyValue("vector-effect") === "non-scaling-stroke") copies[index].style.setProperty("vector-effect", "none");
  });
  copy.setAttribute("width", String(width));
  copy.setAttribute("height", String(height));
  return new XMLSerializer().serializeToString(copy);
}

async function paintSVG(context: CanvasRenderingContext2D, source: string, width: number, height: number): Promise<void> {
  const url = URL.createObjectURL(new Blob([source], { type: "image/svg+xml;charset=utf-8" }));
  try {
    const image = new Image();
    await new Promise<void>((resolve, reject) => {
      image.onload = () => resolve();
      image.onerror = () => reject(new Error("无法保存视口标注"));
      image.src = url;
    });
    context.drawImage(image, 0, 0, width, height);
  } finally { URL.revokeObjectURL(url); }
}

/** Copies the completed frame synchronously, before WebGL discards its buffer.
 * No preserveDrawingBuffer, extra renderer, or change to camera/scene/model state. */
export class ViewportSnapshots {
  private pending: Request[] = [];
  private active = new Set<Request>();
  private disposed = false;

  request(): Promise<Blob> {
    if (this.disposed) return Promise.reject(new Error("视口已关闭"));
    return new Promise((resolve, reject) => {
      const request = { resolve, reject };
      this.pending.push(request); this.active.add(request);
    });
  }

  captureFrame(canvas: HTMLCanvasElement, overlays: Iterable<SVGSVGElement>): void {
    if (!this.pending.length) return;
    const requests = this.pending.splice(0);
    const complete = (blob?: Blob, error?: Error) => {
      for (const request of requests) {
        if (!this.active.delete(request)) continue;
        if (blob) request.resolve(blob); else request.reject(error ?? new Error("截图失败"));
      }
    };
    try {
      if (!canvas.width || !canvas.height) throw new Error("视口尚未就绪");
      const copy = document.createElement("canvas");
      copy.width = canvas.width; copy.height = canvas.height;
      const context = copy.getContext("2d");
      if (!context) throw new Error("无法创建截图");
      context.drawImage(canvas, 0, 0);
      // Freeze labels from this same frame before asynchronous PNG encoding.
      const labels = [...overlays].filter(svg => svg.childElementCount).map(svg => snapshotSVG(svg, copy.width, copy.height));
      void (async () => {
        try {
          for (const label of labels) await paintSVG(context, label, copy.width, copy.height);
          copy.toBlob(blob => blob ? complete(blob) : complete(undefined, new Error("无法保存截图")), "image/png");
        } catch (error) { complete(undefined, error instanceof Error ? error : new Error("截图失败")); }
      })();
    } catch (error) { complete(undefined, error instanceof Error ? error : new Error("截图失败")); }
  }

  dispose(): void {
    this.disposed = true;
    this.cancel(new Error("视口已关闭"));
  }

  cancel(error: Error): void {
    for (const request of this.active) request.reject(error);
    this.active.clear(); this.pending = [];
  }
}

export function saveViewportSnapshot(blob: Blob, documentName: string): void {
  const name = documentName.replace(/[<>:"/\\|?*\x00-\x1f]/g, "_").trim().slice(0, 100) || "occccad";
  const anchor = document.createElement("a");
  const url = URL.createObjectURL(blob);
  anchor.href = url;
  anchor.download = `${name}.png`;
  anchor.click();
  // Allow the browser to start the download before releasing its backing Blob.
  window.setTimeout(() => URL.revokeObjectURL(url), 60_000);
}
