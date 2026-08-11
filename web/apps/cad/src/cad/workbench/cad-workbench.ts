export type CadWorkbenchID = "SKETCHER" | "PART_DESIGN" | "GENERATIVE_SHAPE_DESIGN" | "ASSEMBLY_DESIGN";

export const CAD_WORKBENCHES: Record<CadWorkbenchID, {
  label: string;
  toolbarID: string;
  domain: "SKETCH" | "SOLID" | "SURFACE" | "ASSEMBLY";
}> = {
  SKETCHER: { label: "Sketcher", toolbarID: "sketcher", domain: "SKETCH" },
  PART_DESIGN: { label: "Part Design", toolbarID: "part-design", domain: "SOLID" },
  GENERATIVE_SHAPE_DESIGN: { label: "Generative Shape Design", toolbarID: "surface-design", domain: "SURFACE" },
  ASSEMBLY_DESIGN: { label: "Assembly Design", toolbarID: "assembly-design", domain: "ASSEMBLY" },
};

export function resolveCadWorkbench(documentType: "PART" | "PRODUCT", sketchActive: boolean): CadWorkbenchID {
  if (sketchActive) return "SKETCHER";
  return documentType === "PRODUCT" ? "ASSEMBLY_DESIGN" : "PART_DESIGN";
}
