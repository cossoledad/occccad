import assert from "node:assert/strict";
import { CommandPreviewIdentities } from "../command-preview-identity.ts";

const ids = new CommandPreviewIdentities();
ids.remember("product", "preview", "request", 100);
assert.equal(ids.requestFor("product", "preview", 101), "request");
assert.equal(ids.requestFor("product", "preview", 102), "request", "retry keeps the same identity");
assert.equal(ids.requestFor("other", "preview", 102), undefined);
assert.equal(ids.requestFor("product", "preview", 45_100), undefined);
for (let i = 0; i < 257; ++i) ids.remember("product", `p${i}`, `r${i}`, 200);
assert.equal(ids.requestFor("product", "p0", 201), undefined);
assert.equal(ids.requestFor("product", "p256", 201), "r256");
