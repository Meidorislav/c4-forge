export type Point = Readonly<{ x: number; y: number }>;

type BaseItem = Readonly<{
  id: string;
  position: Point;
  zIndex: number;
}>;

export type C4Item = BaseItem &
  Readonly<{
    kind: "c4";
    shape: "box" | "person" | "cylinder";
    caption: string;
    name: string;
    technology: string;
    description: string;
  }>;

export type FreeformItem = BaseItem &
  Readonly<{
    kind: "sticky" | "rectangle" | "ellipse";
    text: string;
  }>;

export type StrokeItem = BaseItem &
  Readonly<{
    kind: "stroke";
    color: string;
    points: readonly Point[];
  }>;

export type CanvasItem = C4Item | FreeformItem | StrokeItem;

export type Relationship = Readonly<{
  id: string;
  source: string;
  target: string;
  label: string;
  kind: "derived" | "bound";
  parallelIndex: number;
  parallelCount: number;
}>;

export type Presence = Readonly<{
  name: string;
  color: string;
  cursor: Point;
  selection: readonly string[];
}>;

export type CanvasState = Readonly<{
  items: Readonly<Record<string, CanvasItem>>;
  order: readonly string[];
  relationships: readonly Relationship[];
  selection: readonly string[];
  presence: readonly Presence[];
  operationCount: number;
  dataset: "demo" | "performance";
}>;

export type CanvasOperation =
  | Readonly<{ type: "move"; positions: Readonly<Record<string, Point>> }>
  | Readonly<{ type: "select"; ids: readonly string[] }>
  | Readonly<{ type: "update-text"; id: string; text: string }>
  | Readonly<{ type: "add"; item: CanvasItem }>
  | Readonly<{ type: "remove"; ids: readonly string[] }>
  | Readonly<{ type: "change-layer"; ids: readonly string[]; direction: "forward" | "backward" }>
  | Readonly<{ type: "replace"; state: CanvasState }>;

const demoItems: readonly CanvasItem[] = [
  {
    id: "person-architect",
    kind: "c4",
    shape: "person",
    caption: "Person",
    name: "Software Architect",
    technology: "",
    description: "Creates and shares architecture diagrams.",
    position: { x: 60, y: 120 },
    zIndex: 20,
  },
  {
    id: "system-forge",
    kind: "c4",
    shape: "box",
    caption: "Software System",
    name: "c4-forge",
    technology: "Go, React, PostgreSQL",
    description:
      "A self-hosted collaborative tool for modeling software architecture with the C4 model.",
    position: { x: 430, y: 80 },
    zIndex: 20,
  },
  {
    id: "database-model",
    kind: "c4",
    shape: "cylinder",
    caption: "Container: Database",
    name: "Architecture model",
    technology: "PostgreSQL",
    description: "Stores model entities, views and durable operations.",
    position: { x: 860, y: 130 },
    zIndex: 20,
  },
  {
    id: "sticky-question",
    kind: "sticky",
    text: "Double-click the text and type",
    position: { x: 220, y: 430 },
    zIndex: 40,
  },
  {
    id: "freeform-rectangle",
    kind: "rectangle",
    text: "Freeform rectangle",
    position: { x: 570, y: 440 },
    zIndex: 5,
  },
  {
    id: "freeform-ellipse",
    kind: "ellipse",
    text: "Freeform ellipse",
    position: { x: 850, y: 440 },
    zIndex: 35,
  },
  {
    id: "freehand-example",
    kind: "stroke",
    color: "#e11d48",
    points: [
      { x: 4, y: 42 },
      { x: 34, y: 10 },
      { x: 68, y: 54 },
      { x: 102, y: 14 },
      { x: 142, y: 44 },
    ],
    position: { x: 1100, y: 420 },
    zIndex: 45,
  },
];

const demoRelationships: readonly Relationship[] = [
  {
    id: "uses-1",
    source: "person-architect",
    target: "system-forge",
    label: "Creates diagrams with",
    kind: "derived",
    parallelIndex: 0,
    parallelCount: 2,
  },
  {
    id: "uses-2",
    source: "person-architect",
    target: "system-forge",
    label: "Reviews models in",
    kind: "derived",
    parallelIndex: 1,
    parallelCount: 2,
  },
  {
    id: "stores",
    source: "system-forge",
    target: "database-model",
    label: "Stores and loads",
    kind: "derived",
    parallelIndex: 0,
    parallelCount: 1,
  },
  {
    id: "bound-arrow",
    source: "sticky-question",
    target: "system-forge",
    label: "Bound freeform arrow",
    kind: "bound",
    parallelIndex: 0,
    parallelCount: 1,
  },
];

function indexItems(items: readonly CanvasItem[]) {
  return Object.fromEntries(items.map((item) => [item.id, item]));
}

export function createDemoState(): CanvasState {
  return {
    items: indexItems(demoItems),
    order: demoItems.map((item) => item.id),
    relationships: demoRelationships,
    selection: [],
    presence: [
      {
        name: "Morgan",
        color: "#7c3aed",
        cursor: { x: 760, y: 330 },
        selection: ["database-model", "freeform-ellipse"],
      },
    ],
    operationCount: 0,
    dataset: "demo",
  };
}

export function createPerformanceState(): CanvasState {
  const items: CanvasItem[] = Array.from({ length: 300 }, (_, index) => ({
    id: `perf-${index}`,
    kind: "c4" as const,
    shape: index % 17 === 0 ? ("cylinder" as const) : ("box" as const),
    caption: index % 17 === 0 ? "Container: Database" : "Container",
    name: `Service ${index + 1}`,
    technology: index % 3 === 0 ? "Go" : index % 3 === 1 ? "TypeScript" : "PostgreSQL",
    description: "Representative C4 node used by the React Flow performance spike.",
    position: { x: (index % 20) * 270, y: Math.floor(index / 20) * 190 },
    zIndex: 20,
  }));

  const relationships: Relationship[] = Array.from({ length: 400 }, (_, index) => {
    const sourceIndex = index % 300;
    const targetIndex = (sourceIndex + (index < 300 ? 1 : 20)) % 300;
    return {
      id: `perf-edge-${index}`,
      source: `perf-${sourceIndex}`,
      target: `perf-${targetIndex}`,
      label: index < 24 ? `Call ${index + 1}` : "",
      kind: "derived" as const,
      parallelIndex: 0,
      parallelCount: 1,
    };
  });

  return {
    items: indexItems(items),
    order: items.map((item) => item.id),
    relationships,
    selection: items.slice(0, 20).map((item) => item.id),
    presence: [
      {
        name: "Morgan",
        color: "#7c3aed",
        cursor: { x: 930, y: 580 },
        selection: ["perf-43", "perf-44"],
      },
    ],
    operationCount: 0,
    dataset: "performance",
  };
}

export function applyCanvasOperation(state: CanvasState, operation: CanvasOperation): CanvasState {
  if (operation.type === "replace") {
    return operation.state;
  }

  if (operation.type === "select") {
    return { ...state, selection: [...operation.ids], operationCount: state.operationCount + 1 };
  }

  if (operation.type === "move") {
    const items = { ...state.items };
    for (const [id, position] of Object.entries(operation.positions)) {
      const item = items[id];
      if (item) items[id] = { ...item, position };
    }
    return { ...state, items, operationCount: state.operationCount + 1 };
  }

  if (operation.type === "update-text") {
    const item = state.items[operation.id];
    if (!item || (item.kind !== "sticky" && item.kind !== "rectangle" && item.kind !== "ellipse")) {
      return state;
    }
    return {
      ...state,
      items: { ...state.items, [item.id]: { ...item, text: operation.text } },
      operationCount: state.operationCount + 1,
    };
  }

  if (operation.type === "add") {
    return {
      ...state,
      items: { ...state.items, [operation.item.id]: operation.item },
      order: [...state.order, operation.item.id],
      selection: [operation.item.id],
      operationCount: state.operationCount + 1,
    };
  }

  if (operation.type === "remove") {
    const removed = new Set(operation.ids);
    const items = Object.fromEntries(
      Object.entries(state.items).filter(([id]) => !removed.has(id)),
    );
    return {
      ...state,
      items,
      order: state.order.filter((id) => !removed.has(id)),
      relationships: state.relationships.filter(
        (relationship) => !removed.has(relationship.source) && !removed.has(relationship.target),
      ),
      selection: state.selection.filter((id) => !removed.has(id)),
      operationCount: state.operationCount + 1,
    };
  }

  const delta = operation.direction === "forward" ? 10 : -10;
  const items = { ...state.items };
  for (const id of operation.ids) {
    const item = items[id];
    if (item) items[id] = { ...item, zIndex: Math.max(0, item.zIndex + delta) };
  }
  return { ...state, items, operationCount: state.operationCount + 1 };
}

export function selectedC4Item(state: CanvasState): C4Item | undefined {
  for (const id of state.selection) {
    const item = state.items[id];
    if (item?.kind === "c4") return item;
  }
  return state.order
    .map((id) => state.items[id])
    .find((item): item is C4Item => item?.kind === "c4");
}
