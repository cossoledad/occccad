export type Vec2 = [number, number];
export type Vec3 = [number, number, number];
export type PlaneName = "XY" | "XZ" | "YZ";

export type MeshData = {
  vertices: Vec3[];
  triangles: [number, number, number][];
  faceIds: number[];
};

export type Artifact = {
  geometryKey: string;
  geometryId: string;
  mesh: MeshData;
  bbox: { min: Vec3; max: Vec3 };
  topology: { faces: number; edges: number; vertices: number; solids: number };
  volume: number;
  occtVersion: string;
  glbBytes: number;
};

export type DatumPlane = { id: string; name: string; plane: PlaneName };

export type Feature = {
  id: string;
  type: "RECTANGLE_SKETCH" | "rectangle_sketch" | "PAD" | "pad";
  name?: string;
  plane?: PlaneName;
  rectangle?: { origin: Vec2; width: number; height: number };
  origin?: Vec2;
  width?: number;
  height?: number;
  profile?: string;
  length?: number;
};

export type ProductInstance = {
  id: string;
  name: string;
  documentId: string;
  versionId: string;
  translation: Vec3;
  referenceMode?: "FOLLOW_HEAD" | "PINNED";
  resolvedVersionId?: string;
  headChanged?: boolean;
};

export type DocumentSummary = {
  id: string;
  name: string;
  type: "PART" | "PRODUCT";
  versionId: string;
  canUndo: boolean;
  canRedo: boolean;
  lastUpdated: string;
};

export type ResolvedInstance = {
  id: string;
  name: string;
  documentId: string;
  geometryKey: string;
  translation: Vec3;
};

export type DocumentView = {
  document: DocumentSummary;
  datumPlanes?: DatumPlane[];
  part?: { units: string; features: Feature[] };
  product?: { instances: ProductInstance[] };
  artifact?: Artifact;
  artifacts?: Record<string, Artifact>;
  resolvedInstances?: ResolvedInstance[];
};

export type Selection =
  | { kind: "plane"; id: string; plane: PlaneName }
  | { kind: "sketch"; id: string }
  | { kind: "pad"; id: string }
  | { kind: "instance"; id: string }
  | { kind: "solid"; id: string }
  | null;

export type RectangleDraft = {
  plane: PlaneName;
  origin: Vec2;
  width: number;
  height: number;
};
