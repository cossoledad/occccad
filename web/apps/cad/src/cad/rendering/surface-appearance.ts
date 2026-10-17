import * as THREE from "three";
import type { ResolvedAppearance } from "../../types";

// Keep index order intact: faceIndex remains the exact GLB triangle locator.
export function surfaceAppearanceMaterials(
  geometry: THREE.BufferGeometry, faceIds: readonly number[], appearance: ResolvedAppearance | undefined,
  defaultColor: number, create: (color: number) => THREE.Material,
): THREE.Material | THREE.Material[] {
  const base = appearance?.color ? Number.parseInt(appearance.color.slice(1), 16) : defaultColor;
  if (!appearance?.faces || !Object.keys(appearance.faces).length) return create(base);
  const colors: number[] = [], indices = new Map<number, number>();
  let previous = -1;
  faceIds.forEach((id, triangle) => {
    const value = appearance.faces![id];
    const color = value ? Number.parseInt(value.slice(1), 16) : base;
    let material = indices.get(color);
    if (material === undefined) { material = colors.length; colors.push(color); indices.set(color, material); }
    if (previous === material) geometry.groups.at(-1)!.count += 3;
    else { geometry.addGroup(triangle * 3, 3, material); previous = material; }
  });
  return colors.map(create);
}
