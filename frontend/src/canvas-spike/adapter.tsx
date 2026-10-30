import {
  BaseEdge,
  type Edge,
  EdgeLabelRenderer,
  type EdgeProps,
  Handle,
  type Node,
  type NodeChange,
  type NodeProps,
  Position,
} from "@xyflow/react";
import type { CSSProperties } from "react";
import { strokePath } from "./freehand";
import type { CanvasItem, CanvasState, Relationship } from "./model";
import type { CanvasStore } from "./store";

type SpikeNodeData = {
  item: CanvasItem;
  remoteColors: readonly string[];
  apply: CanvasStore["apply"];
};

export type SpikeNode = Node<SpikeNodeData, CanvasItem["kind"]>;

type SpikeEdgeData = {
  relationship: Relationship;
};

export type SpikeEdge = Edge<SpikeEdgeData, "relationship">;

type NodeCacheEntry = {
  item: CanvasItem;
  remoteKey: string;
  selected: boolean;
  node: SpikeNode;
};

type EdgeCacheEntry = {
  relationship: Relationship;
  sourcePosition: string;
  targetPosition: string;
  edge: SpikeEdge;
};

export type AdapterCache = {
  nodes: Map<string, NodeCacheEntry>;
  edges: Map<string, EdgeCacheEntry>;
};

export function createAdapterCache(): AdapterCache {
  return { nodes: new Map(), edges: new Map() };
}

const handlePositions = [Position.Top, Position.Right, Position.Bottom, Position.Left] as const;

function Handles() {
  return handlePositions.flatMap((position) => [
    <Handle
      className="spike-handle"
      id={`source-${position}`}
      key={`source-${position}`}
      position={position}
      type="source"
    />,
    <Handle
      className="spike-handle"
      id={`target-${position}`}
      key={`target-${position}`}
      position={position}
      type="target"
    />,
  ]);
}

function RemoteRings({ colors }: { colors: readonly string[] }) {
  return colors.map((color) => (
    <span className="spike-remote-ring" key={color} style={{ borderColor: color }} />
  ));
}

function C4Node({ data }: NodeProps<SpikeNode>) {
  if (data.item.kind !== "c4") return null;
  const item = data.item;
  return (
    <div className={`spike-c4-node spike-c4-node--${item.shape}`}>
      <RemoteRings colors={data.remoteColors} />
      {item.shape === "person" && <span aria-hidden="true" className="spike-person-icon" />}
      {item.shape === "cylinder" && <span aria-hidden="true" className="spike-cylinder-top" />}
      <span className="spike-node-caption">{item.caption}</span>
      <strong>{item.name}</strong>
      {item.technology && <span className="spike-node-technology">[{item.technology}]</span>}
      <span className="spike-node-description">{item.description}</span>
      <Handles />
    </div>
  );
}

function FreeformNode({ data }: NodeProps<SpikeNode>) {
  const item = data.item;
  if (item.kind !== "sticky" && item.kind !== "rectangle" && item.kind !== "ellipse") return null;
  return (
    <div className={`spike-freeform spike-freeform--${item.kind}`}>
      <RemoteRings colors={data.remoteColors} />
      <textarea
        aria-label={item.kind}
        className="nodrag nowheel"
        onChange={(event) =>
          data.apply({ type: "update-text", id: item.id, text: event.currentTarget.value })
        }
        value={item.text}
      />
      <Handles />
    </div>
  );
}

function StrokeNode({ data }: NodeProps<SpikeNode>) {
  if (data.item.kind !== "stroke") return null;
  const item = data.item;
  const width = Math.max(16, ...item.points.map((point) => point.x + 8));
  const height = Math.max(16, ...item.points.map((point) => point.y + 8));
  return (
    <div className="spike-stroke" style={{ height, width }}>
      <RemoteRings colors={data.remoteColors} />
      <svg aria-hidden="true" height={height} viewBox={`0 0 ${width} ${height}`} width={width}>
        <path d={strokePath(item.points)} fill={item.color} />
      </svg>
      <Handles />
    </div>
  );
}

function relationshipPath(
  sourceX: number,
  sourceY: number,
  targetX: number,
  targetY: number,
  relationship: Relationship,
) {
  const dx = targetX - sourceX;
  const dy = targetY - sourceY;
  const length = Math.max(1, Math.hypot(dx, dy));
  const offset = (relationship.parallelIndex - (relationship.parallelCount - 1) / 2) * 58;
  const middleX = (sourceX + targetX) / 2 + (-dy / length) * offset;
  const middleY = (sourceY + targetY) / 2 + (dx / length) * offset;
  return {
    path: `M ${sourceX} ${sourceY} Q ${middleX} ${middleY} ${targetX} ${targetY}`,
    labelX: 0.25 * sourceX + 0.5 * middleX + 0.25 * targetX,
    labelY: 0.25 * sourceY + 0.5 * middleY + 0.25 * targetY,
  };
}

function RelationshipEdge({
  data,
  markerEnd,
  sourceX,
  sourceY,
  targetX,
  targetY,
}: EdgeProps<SpikeEdge>) {
  if (!data) return null;
  const { relationship } = data;
  const { path, labelX, labelY } = relationshipPath(
    sourceX,
    sourceY,
    targetX,
    targetY,
    relationship,
  );
  return (
    <>
      <BaseEdge
        className={`spike-edge spike-edge--${relationship.kind}`}
        markerEnd={markerEnd}
        path={path}
      />
      {relationship.label && (
        <EdgeLabelRenderer>
          <span
            className="spike-edge-label nodrag nopan"
            style={{ transform: `translate(-50%, -50%) translate(${labelX}px, ${labelY}px)` }}
          >
            {relationship.label}
          </span>
        </EdgeLabelRenderer>
      )}
    </>
  );
}

export const nodeTypes = {
  c4: C4Node,
  sticky: FreeformNode,
  rectangle: FreeformNode,
  ellipse: FreeformNode,
  stroke: StrokeNode,
};

export const edgeTypes = { relationship: RelationshipEdge };

function itemStyle(item: CanvasItem): CSSProperties {
  switch (item.kind) {
    case "c4":
      return { width: item.shape === "person" ? 245 : 300 };
    case "sticky":
      return { height: 150, width: 220 };
    case "rectangle":
      return { height: 125, width: 230 };
    case "ellipse":
      return { height: 140, width: 230 };
    case "stroke":
      return {};
  }
}

export function stateToNodes(
  state: CanvasState,
  apply: CanvasStore["apply"],
  cache?: AdapterCache,
): SpikeNode[] {
  const remoteColors = new Map<string, string[]>();
  for (const participant of state.presence) {
    for (const id of participant.selection) {
      remoteColors.set(id, [...(remoteColors.get(id) ?? []), participant.color]);
    }
  }

  return state.order.flatMap((id) => {
    const item = state.items[id];
    if (!item) return [];
    const colors = remoteColors.get(id) ?? [];
    const remoteKey = colors.join("\0");
    const selected = state.selection.includes(id);
    const cached = cache?.nodes.get(id);
    if (
      cached &&
      cached.item === item &&
      cached.remoteKey === remoteKey &&
      cached.selected === selected
    ) {
      return [cached.node];
    }
    const node: SpikeNode = {
      id: item.id,
      type: item.kind,
      position: item.position,
      data: { item, remoteColors: colors, apply },
      selected,
      zIndex: item.zIndex,
      style: itemStyle(item),
    };
    cache?.nodes.set(id, { item, remoteKey, selected, node });
    return [node];
  });
}

function handlePair(source: CanvasItem, target: CanvasItem) {
  const dx = target.position.x - source.position.x;
  const dy = target.position.y - source.position.y;
  if (Math.abs(dx) >= Math.abs(dy)) {
    return dx >= 0
      ? { sourceHandle: "source-right", targetHandle: "target-left" }
      : { sourceHandle: "source-left", targetHandle: "target-right" };
  }
  return dy >= 0
    ? { sourceHandle: "source-bottom", targetHandle: "target-top" }
    : { sourceHandle: "source-top", targetHandle: "target-bottom" };
}

export function stateToEdges(state: CanvasState, cache?: AdapterCache): SpikeEdge[] {
  return state.relationships.flatMap((relationship) => {
    const source = state.items[relationship.source];
    const target = state.items[relationship.target];
    if (!source || !target) return [];
    const sourcePosition = `${source.position.x}:${source.position.y}`;
    const targetPosition = `${target.position.x}:${target.position.y}`;
    const cached = cache?.edges.get(relationship.id);
    if (
      cached &&
      cached.relationship === relationship &&
      cached.sourcePosition === sourcePosition &&
      cached.targetPosition === targetPosition
    ) {
      return [cached.edge];
    }
    const edge: SpikeEdge = {
      id: relationship.id,
      source: relationship.source,
      target: relationship.target,
      type: "relationship",
      data: { relationship },
      ...handlePair(source, target),
      markerEnd: {
        type: "arrowclosed",
        color: relationship.kind === "bound" ? "#c2410c" : "#334155",
      },
      zIndex: relationship.kind === "bound" ? 30 : 10,
    };
    cache?.edges.set(relationship.id, {
      relationship,
      sourcePosition,
      targetPosition,
      edge,
    });
    return [edge];
  });
}

export function applyNodeChangesToStore(
  changes: readonly NodeChange<SpikeNode>[],
  state: CanvasState,
  apply: CanvasStore["apply"],
) {
  const positions: Record<string, { x: number; y: number }> = {};
  const selection = new Set(state.selection);
  const removed: string[] = [];

  for (const change of changes) {
    if (change.type === "position" && change.position) positions[change.id] = change.position;
    if (change.type === "select") {
      if (change.selected) selection.add(change.id);
      else selection.delete(change.id);
    }
    if (change.type === "remove") removed.push(change.id);
  }

  if (Object.keys(positions).length > 0) apply({ type: "move", positions });
  if ([...selection].join("\0") !== state.selection.join("\0")) {
    apply({ type: "select", ids: [...selection] });
  }
  if (removed.length > 0) apply({ type: "remove", ids: removed });
}
