type RandomSource = {
  randomUUID?: () => string;
  getRandomValues?: (array: Uint8Array) => Uint8Array;
};

const defaultRandomSource: RandomSource = typeof globalThis.crypto === "undefined"
  ? {}
  : globalThis.crypto;

/**
 * randomUUID() is restricted to secure contexts, while getRandomValues() is
 * available for the HTTP LAN development URL used by the WSL2 setup.
 */
export function randomUUID(source: RandomSource = defaultRandomSource): string {
  if (typeof source.randomUUID === "function") return source.randomUUID();
  if (typeof source.getRandomValues !== "function") {
    throw new Error("secure random UUID generation is unavailable");
  }

  const bytes = source.getRandomValues(new Uint8Array(16));
  bytes[6] = (bytes[6] & 0x0f) | 0x40;
  bytes[8] = (bytes[8] & 0x3f) | 0x80;
  const hex = Array.from(bytes, (byte) => byte.toString(16).padStart(2, "0")).join("");
  return `${hex.slice(0, 8)}-${hex.slice(8, 12)}-${hex.slice(12, 16)}-${hex.slice(16, 20)}-${hex.slice(20)}`;
}
