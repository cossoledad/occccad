import assert from "node:assert/strict";
import { createServer } from "vite";

const server = await createServer({ appType: "custom", logLevel: "silent", server: { middlewareMode: true } });
const originalDocument = globalThis.document;
try {
  const { ViewportSnapshots } = await server.ssrLoadModule("/src/viewport/viewport-snapshot.ts");
  const encodings = [], copiedFrames = [];
  globalThis.document = { createElement: () => ({
    width: 0, height: 0,
    getContext: () => ({ drawImage: frame => copiedFrames.push(frame) }),
    toBlob: callback => encodings.push(callback),
  }) };
  const queue = new ViewportSnapshots();
  const first = queue.request(), sameFrame = queue.request(), frame = { width: 1600, height: 900 };
  assert.equal(copiedFrames.length, 0, "an idle/discarded WebGL buffer is never copied before render");
  queue.captureFrame(frame, []);
  const nextFrame = queue.request(); queue.captureFrame({ width: 800, height: 600 }, []);
  assert.equal(copiedFrames.length, 2, "a later request needs its own completed frame");
  const nextBlob = new Blob(["new frame"], { type: "image/png" });
  encodings[1](nextBlob); assert.equal(await nextFrame, nextBlob, "PNG encoders may complete out of order");
  const firstBlob = new Blob(["first frame"], { type: "image/png" });
  encodings[0](firstBlob); assert.equal(await first, firstBlob); assert.equal(await sameFrame, firstBlob);
  const lost = queue.request(), rejected = assert.rejects(lost, /context lost/);
  queue.cancel(new Error("context lost")); await rejected;
  const recovered = queue.request(); queue.captureFrame(frame, []); encodings[2](firstBlob); assert.equal(await recovered, firstBlob);
  const pending = queue.request(), encoding = queue.request(); queue.captureFrame(frame, []);
  const stopped = [assert.rejects(pending, /视口已关闭/), assert.rejects(encoding, /视口已关闭/)];
  queue.dispose(); encodings[3](firstBlob); await Promise.all(stopped); await assert.rejects(queue.request(), /视口已关闭/);
  const invalid = new ViewportSnapshots(), empty = invalid.request();
  invalid.captureFrame({ width: 0, height: 0 }, []); await assert.rejects(empty, /尚未就绪/);
} finally { globalThis.document = originalDocument; await server.close(); }
