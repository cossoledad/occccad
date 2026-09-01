import type { ReactNode } from "react";

export type CadIconName =
  | "select" | "capture" | "sketch" | "pad" | "pocket" | "revolve" | "datum-plane" | "datum-axis" | "point" | "line" | "circle" | "arc" | "polyline" | "spline" | "finish"
  | "coincident" | "parallel" | "fixed" | "horizontal" | "vertical" | "perpendicular" | "tangent" | "equal"
  | "distance" | "length" | "radius" | "diameter" | "angle" | "concentric" | "point-on-object" | "midpoint" | "symmetry"
  | "rectangle" | "polygon" | "slot" | "insert" | "reference" | "link" | "undo" | "redo" | "version" | "share"
  | "navigation" | "fit" | "isometric" | "debug";

const P = ({ d }: { d: string }) => <path d={d} />;
const C = ({ cx, cy, r = 1.5 }: { cx: number; cy: number; r?: number }) => <circle cx={cx} cy={cy} r={r} />;

function glyph(name: CadIconName): ReactNode {
  switch (name) {
  case "select": return <><P d="M5 3l10 8-5 .8-2.8 4.6z" /><P d="M10.2 11.8l4.2 4.2" /></>;
  case "capture": return <><P d="M4 7V4h3M13 4h3v3M16 13v3h-3M7 16H4v-3" /><C cx={10} cy={10} r={2.2} /><P d="M10 2v3M10 15v3M2 10h3M15 10h3" /></>;
  case "sketch": return <><P d="M3 16l4-11h7l3 11zM5 12h10" /><P d="M7 5l3 7 4-7" /></>;
  case "pad": return <><P d="M4 12l6 3.5 6-3.5-6-3.5zM4 12v3l6 3 6-3v-3M10 8.5V3" /><P d="M7.5 5.5L10 3l2.5 2.5" /></>;
  case "pocket": return <><P d="M4 7l6 3.5L16 7M4 7v7l6 3 6-3V7" /><P d="M10 3v7M7.5 7.5L10 10l2.5-2.5" /></>;
  case "revolve": return <><P d="M6 15a7 7 0 116 1M6 15V9M6 15h6" /><P d="M10 4v12" /></>;
  case "datum-plane": return <><P d="M3 13l10-8 4 3-10 8z" /><P d="M10 3v14M6 10h8" /></>;
  case "datum-axis": return <><P d="M3 10h14M10 3v14" /><C cx={10} cy={10} r={2} /></>;
  case "point": return <><C cx={10} cy={10} r={2.2} /><P d="M10 3v3M10 14v3M3 10h3M14 10h3" /></>;
  case "line": return <><P d="M4 16L16 4" /><C cx={4} cy={16} /><C cx={16} cy={4} /></>;
  case "circle": return <><C cx={10} cy={10} r={6} /><C cx={10} cy={10} r={1} /><P d="M10 10l4-4" /></>;
  case "arc": return <><P d="M4 15A9 9 0 0116 5" /><C cx={4} cy={15} /><C cx={16} cy={5} /><P d="M10 10l6-5" /></>;
  case "polyline": return <><P d="M3 15l4-8 5 5 5-8" /><C cx={3} cy={15} /><C cx={7} cy={7} /><C cx={12} cy={12} /><C cx={17} cy={4} /></>;
  case "spline": return <><P d="M3 14C6 2 13 18 17 6" /><C cx={3} cy={14} /><C cx={17} cy={6} /></>;
  case "finish": return <P d="M4 10l4 4 8-9" />;
  case "coincident": return <><C cx={7} cy={10} r={3.2} /><C cx={13} cy={10} r={3.2} /><C cx={10} cy={10} r={1} /></>;
  case "parallel": return <><P d="M5 15L10 5M10 15l5-10" /><P d="M3 11l2 4 4-1M11 6l4-1 2 4" /></>;
  case "fixed": return <><P d="M5 9h10v8H5zM7 9V7a3 3 0 016 0v2" /><P d="M10 12v2" /></>;
  case "horizontal": return <><P d="M3 10h14M5 7l-2 3 2 3M15 7l2 3-2 3" /></>;
  case "vertical": return <><P d="M10 3v14M7 5l3-2 3 2M7 15l3 2 3-2" /></>;
  case "perpendicular": return <><P d="M4 15h12M10 15V4" /><P d="M10 11h4v4" /></>;
  case "tangent": return <><C cx={10} cy={8} r={4.5} /><P d="M3 14h14M10 12.5V14" /></>;
  case "equal": return <><P d="M4 7h12M4 13h12" /><P d="M7 5v4M13 11v4" /></>;
  case "distance": return <><P d="M4 6v8M16 6v8M4 10h12M7 8l-3 2 3 2M13 8l3 2-3 2" /></>;
  case "length": return <><P d="M4 15L16 5M6 4L3 7M17 13l-3 3M6 7l8 7" /></>;
  case "radius": return <><C cx={9} cy={11} r={6} /><P d="M9 11l5-4M10 7h4v4" /></>;
  case "diameter": return <><C cx={10} cy={10} r={6} /><P d="M5 15L15 5M7 5l8 8" /></>;
  case "angle": return <><P d="M4 15h12M4 15l8-10M8 15a4 4 0 012-3" /></>;
  case "concentric": return <><C cx={10} cy={10} r={6} /><C cx={10} cy={10} r={3} /><C cx={10} cy={10} r={0.7} /></>;
  case "point-on-object": return <><P d="M3 14C7 7 12 15 17 6" /><C cx={10} cy={10} r={2} /></>;
  case "midpoint": return <><P d="M3 14L17 6" /><C cx={10} cy={10} r={2} /><P d="M4 10l-1 4 4 1M13 5l4 1-1 4" /></>;
  case "symmetry": return <><P d="M10 2v16" /><C cx={5} cy={8} r={1.7} /><C cx={15} cy={8} r={1.7} /><P d="M6.7 8h6.6M8 6l2 2-2 2M12 6l-2 2 2 2" /></>;
  case "rectangle": return <><rect x="4" y="5" width="12" height="10" /><C cx={4} cy={5} /><C cx={16} cy={15} /></>;
  case "polygon": return <><P d="M10 3l6 3.5v7L10 17l-6-3.5v-7z" /><C cx={10} cy={3} /></>;
  case "slot": return <><P d="M7 5h6a5 5 0 010 10H7A5 5 0 017 5z" /><P d="M7 5a5 5 0 000 10M13 5a5 5 0 010 10" /></>;
  case "insert": return <><P d="M4 7l6-3 6 3-6 3zM4 7v6l6 3 6-3V7" /><P d="M10 7v7M13 11h5M15.5 8.5v5" /></>;
  case "reference": return <><P d="M6 5h8v10H6zM3 8h3M14 12h3" /><P d="M3 8l2-2M3 8l2 2M17 12l-2-2M17 12l-2 2" /></>;
  case "link": return <><P d="M8 7l-1.5-1.5a3 3 0 00-4.2 4.2l2 2a3 3 0 004.2 0l2-2"/><P d="M12 13l1.5 1.5a3 3 0 004.2-4.2l-2-2a3 3 0 00-4.2 0l-2 2"/></>;
  case "undo": return <><P d="M7 6L3 10l4 4M4 10h7a5 5 0 015 5" /></>;
  case "redo": return <><P d="M13 6l4 4-4 4M16 10H9a5 5 0 00-5 5" /></>;
  case "version": return <><P d="M5 3h8l3 3v11H5zM8 3v5h5V3M8 14h5" /></>;
  case "share": return <><C cx={5} cy={10} r={2} /><C cx={15} cy={5} r={2} /><C cx={15} cy={15} r={2} /><P d="M7 9l6-3M7 11l6 3" /></>;
  case "navigation": return <><C cx={10} cy={10} r={7} /><P d="M12.5 7.5l-1.5 4-4 1.5 1.5-4z" /></>;
  case "fit": return <><P d="M8 4H4v4M12 4h4v4M4 12v4h4M16 12v4h-4" /><rect x="7" y="7" width="6" height="6" /></>;
  case "isometric": return <><P d="M10 3l6 3.5v7L10 17l-6-3.5v-7zM4 6.5l6 3.5 6-3.5M10 10v7" /></>;
	case "debug": return <><P d="M7 7h6a3 3 0 013 3v3a6 6 0 01-12 0v-3a3 3 0 013-3zM10 7V4M7 4l3 3 3-3" /><P d="M4 10H2M18 10h-2M4 14H2M18 14h-2" /></>;
  }
}

export function CadIcon({ name }: { name: CadIconName }) {
  return <svg className="cad-command-icon" viewBox="0 0 20 20" width="20" height="20" aria-hidden="true"
    fill="none" stroke="currentColor" strokeWidth="1.45" strokeLinecap="round" strokeLinejoin="round">
    {glyph(name)}
  </svg>;
}
