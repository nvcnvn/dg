// Code generated from the operation definitions of internal/ops by
// `go test ./internal/ops -run TestTypeScript -update`. DO NOT EDIT.

// Op is an operation (spec FR-12) as the editor sends it.
export type Op =
  | { op: "create_node"; kind: string; id?: string; label?: string; parent?: string; view?: string; x?: number; y?: number; before?: string }
  | { op: "delete_node"; id: string; recursive?: boolean }
  | { op: "set_label"; id: string; label: string }
  | { op: "set_parent"; id: string; parent?: string }
  | { op: "set_kind"; id: string; kind: string; type?: "node" | "edge" }
  | { op: "set_tags"; id: string; tags: string[]; type?: "node" | "edge" }
  | { op: "set_props"; id: string; props: Record<string, string | number | boolean> }
  | { op: "add_port"; id: string; name: string; side?: "top" | "right" | "bottom" | "left" | "auto" }
  | { op: "remove_port"; id: string; name: string }
  | { op: "set_rows"; id: string; compartments: ({ name?: string; rows: ({ key?: string; name: string; type?: string })[] })[] }
  | { op: "connect"; from: { node: string; port?: string; row?: string }; to: { node: string; port?: string; row?: string }; id?: string; kind?: string; labels?: { end?: string; mid?: string; start?: string }; tags?: string[]; view?: string }
  | { op: "disconnect"; id: string }
  | { op: "set_edge_labels"; id: string; labels?: { end?: string; mid?: string; start?: string } }
  | { op: "move"; view: string; id: string; x: number; y: number }
  | { op: "resize"; view: string; id: string; w: number; h: number }
  | { op: "reorder"; view: string; container: string; order?: string[] }
  | { op: "unpin"; view: string; id: string }
  | { op: "pin_port"; view: string; id: string; port: string; side: "top" | "right" | "bottom" | "left"; offset?: number }
  | { op: "set_edge_hint"; view: string; id: string; hint: { from_side?: "top" | "right" | "bottom" | "left"; label_pos?: number; to_side?: "top" | "right" | "bottom" | "left"; via?: ([number, number])[] } }
  | { op: "rename_id"; type: "node" | "edge" | "view"; old: string; new: string }
  | { op: "create_view"; id: string; title?: string; include?: string[]; exclude?: string[]; collapse?: string[]; edges?: string[]; of?: string; direction?: "down" | "right" | "up" | "left"; overlays?: ({ id: string; label?: string; members: string[]; style?: string })[]; styles?: ({ label: string; props: Record<string, string | number | boolean | number[]>; type?: "style" | "edge_style" })[] }
  | { op: "update_view"; id: string; title?: string; include?: string[]; exclude?: string[]; collapse?: string[]; edges?: string[]; of?: string; direction?: "down" | "right" | "up" | "left"; overlays?: ({ id: string; label?: string; members: string[]; style?: string })[]; styles?: ({ label: string; props: Record<string, string | number | boolean | number[]>; type?: "style" | "edge_style" })[]; unset?: string[] }
  | { op: "delete_view"; id: string }
  | { op: "show_node"; view: string; id: string }
  | { op: "hide_node"; view: string; id: string }
  | { op: "add_decoration"; view: string; id?: string; decoration: { attach?: string; dx?: number; dy?: number; fill?: string; font_size?: number; h?: number; in?: string; opacity?: number; path?: string; points?: ([number, number])[]; src?: string; stroke?: string; stroke_dash?: number[]; stroke_width?: number; text?: string; text_color?: string; type?: "text" | "rect" | "ellipse" | "band" | "image" | "arrow" | "freehand"; w?: number; x?: number; y?: number; z?: number } }
  | { op: "update_decoration"; view: string; id: string; decoration?: { attach?: string; dx?: number; dy?: number; fill?: string; font_size?: number; h?: number; in?: string; opacity?: number; path?: string; points?: ([number, number])[]; src?: string; stroke?: string; stroke_dash?: number[]; stroke_width?: number; text?: string; text_color?: string; type?: "text" | "rect" | "ellipse" | "band" | "image" | "arrow" | "freehand"; w?: number; x?: number; y?: number; z?: number }; unset?: string[] }
  | { op: "delete_decoration"; view: string; id: string }
  | { op: "promote_decoration"; view: string; id: string; kind: string; new?: string; label?: string }
  | { op: "batch"; ops: Op[] };

// OpName names an operation.
export type OpName = Op["op"];

// RESERVED are the IDs that cannot be simple IDs (spec 6.1).
export const RESERVED: readonly string[] = ["all", "node", "kind", "tag", "children", "descendants", "edge_kind", "port", "row", "lift"];
