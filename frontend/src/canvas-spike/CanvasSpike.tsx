import {
  Background,
  BackgroundVariant,
  Controls,
  getViewportForBounds,
  MiniMap,
  ReactFlow,
  ReactFlowProvider,
  SelectionMode,
  useReactFlow,
  ViewportPortal,
} from "@xyflow/react";
import { toPng } from "html-to-image";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import {
  applyNodeChangesToStore,
  createAdapterCache,
  edgeTypes,
  nodeTypes,
  type SpikeNode,
  stateToEdges,
  stateToNodes,
} from "./adapter";
import { downloadText, serializeC4NodeSvg } from "./export";
import {
  createDemoState,
  createPerformanceState,
  type FreeformItem,
  type Point,
  type StrokeItem,
  selectedC4Item,
} from "./model";
import { CanvasStore, useCanvasStore } from "./store";

type Benchmark =
  | { kind: "idle" }
  | { kind: "running" }
  | { kind: "done"; dragFps: number; panFps: number; slow: number };

function downloadDataUrl(filename: string, url: string) {
  const separator = url.indexOf(",");
  if (separator < 0) throw new Error("invalid data URL");
  const metadata = url.slice(0, separator);
  const payload = url.slice(separator + 1);
  const binary = metadata.endsWith(";base64") ? atob(payload) : decodeURIComponent(payload);
  const bytes = Uint8Array.from(binary, (character) => character.charCodeAt(0));
  const mimeType = /^data:([^;]+)/.exec(metadata)?.[1] ?? "application/octet-stream";
  const objectUrl = URL.createObjectURL(new Blob([bytes], { type: mimeType }));
  const anchor = document.createElement("a");
  anchor.download = filename;
  anchor.href = objectUrl;
  anchor.click();
  URL.revokeObjectURL(objectUrl);
}

function Canvas({ store }: { store: CanvasStore }) {
  const { t } = useTranslation();
  const state = useCanvasStore(store);
  const flow = useReactFlow<SpikeNode>();
  const canvasRef = useRef<HTMLDivElement>(null);
  const itemSequence = useRef(0);
  const animationFrame = useRef<number | undefined>(undefined);
  const adapterCache = useRef(createAdapterCache());
  const [penActive, setPenActive] = useState(false);
  const [draft, setDraft] = useState<readonly Point[]>([]);
  const draftRef = useRef<readonly Point[]>([]);
  const [benchmark, setBenchmark] = useState<Benchmark>({ kind: "idle" });
  const [exportMessage, setExportMessage] = useState("");
  const nodes = useMemo(
    () => stateToNodes(state, store.apply, adapterCache.current),
    [state, store.apply],
  );
  const edges = useMemo(() => stateToEdges(state, adapterCache.current), [state]);

  useEffect(
    () => () => {
      if (animationFrame.current !== undefined) cancelAnimationFrame(animationFrame.current);
    },
    [],
  );

  const switchDataset = useCallback(() => {
    const loadingPerformanceDataset = state.dataset === "demo";
    store.apply({
      type: "replace",
      state: loadingPerformanceDataset ? createPerformanceState() : createDemoState(),
    });
    setBenchmark({ kind: "idle" });
    setExportMessage("");
    window.setTimeout(() => {
      if (loadingPerformanceDataset) {
        void flow.setViewport({ x: 300, y: 170, zoom: 0.3 }, { duration: 350 });
      } else {
        void flow.fitView({ duration: 350, padding: 0.12 });
      }
    }, 0);
  }, [flow, state.dataset, store]);

  const addShape = useCallback(
    (kind: FreeformItem["kind"]) => {
      itemSequence.current += 1;
      const bounds = canvasRef.current?.getBoundingClientRect();
      const position = flow.screenToFlowPosition({
        x: (bounds?.left ?? 0) + (bounds?.width ?? 900) / 2,
        y: (bounds?.top ?? 0) + (bounds?.height ?? 600) / 2,
      });
      store.apply({
        type: "add",
        item: {
          id: `${kind}-${Date.now()}-${itemSequence.current}`,
          kind,
          text: t(`spike.newShape.${kind}`),
          position,
          zIndex: kind === "sticky" ? 40 : 15,
        },
      });
    },
    [flow, store, t],
  );

  const onPenPointerDown = useCallback(
    (event: React.PointerEvent<HTMLDivElement>) => {
      event.currentTarget.setPointerCapture(event.pointerId);
      const point = flow.screenToFlowPosition({ x: event.clientX, y: event.clientY });
      draftRef.current = [point];
      setDraft([point]);
    },
    [flow],
  );

  const onPenPointerMove = useCallback(
    (event: React.PointerEvent<HTMLDivElement>) => {
      if (!event.currentTarget.hasPointerCapture(event.pointerId)) return;
      const point = flow.screenToFlowPosition({ x: event.clientX, y: event.clientY });
      draftRef.current = [...draftRef.current, point];
      setDraft(draftRef.current);
    },
    [flow],
  );

  const onPenPointerUp = useCallback(
    (event: React.PointerEvent<HTMLDivElement>) => {
      if (!event.currentTarget.hasPointerCapture(event.pointerId)) return;
      event.currentTarget.releasePointerCapture(event.pointerId);
      const points = draftRef.current;
      draftRef.current = [];
      setDraft([]);
      if (points.length < 2) return;
      const minX = Math.min(...points.map((point) => point.x));
      const minY = Math.min(...points.map((point) => point.y));
      const item: StrokeItem = {
        id: `stroke-${Date.now()}`,
        kind: "stroke",
        color: "#e11d48",
        position: { x: minX, y: minY },
        points: points.map((point) => ({ x: point.x - minX, y: point.y - minY })),
        zIndex: 45,
      };
      store.apply({ type: "add", item });
    },
    [store],
  );

  const exportPng = useCallback(async () => {
    const viewport = canvasRef.current?.querySelector<HTMLElement>(".react-flow__viewport");
    if (!viewport || nodes.length === 0) return;
    try {
      setExportMessage(t("spike.export.rendering"));
      const bounds = flow.getNodesBounds(nodes);
      const width = Math.ceil(bounds.width + 160);
      const height = Math.ceil(bounds.height + 160);
      const transform = getViewportForBounds(bounds, width, height, 0.05, 2, 0.9);
      const dataUrl = await toPng(viewport, {
        backgroundColor: "#f8fafc",
        height,
        width,
        style: {
          height: `${height}px`,
          width: `${width}px`,
          transform: `translate(${transform.x}px, ${transform.y}px) scale(${transform.zoom})`,
        },
      });
      downloadDataUrl("c4-forge-react-flow-spike.png", dataUrl);
      setExportMessage(t("spike.export.ready"));
    } catch {
      setExportMessage(t("spike.export.failed"));
    }
  }, [flow, nodes, t]);

  const exportSvg = useCallback(() => {
    const item = selectedC4Item(state);
    if (!item) return;
    downloadText("c4-node.svg", "image/svg+xml", serializeC4NodeSvg(item));
    setExportMessage(t("spike.export.svgReady"));
  }, [state, t]);

  const runBenchmark = useCallback(() => {
    if (benchmark.kind === "running") return;
    const ids = state.selection.slice(0, 20);
    const baseline = Object.fromEntries(
      ids.flatMap((id) => {
        const item = state.items[id];
        return item ? [[id, item.position]] : [];
      }),
    );
    if (ids.length === 0) return;
    setBenchmark({ kind: "running" });
    const started = performance.now();
    let previous = started;
    let slow = 0;
    const initialViewport = flow.getViewport();
    let phase: "drag" | "pan" = "drag";
    let phaseStarted = started;
    let dragFrames = 0;
    let panFrames = 0;

    const frame = (now: number) => {
      if (now - previous > 25) slow += 1;
      previous = now;
      const phaseElapsed = now - phaseStarted;

      if (phase === "drag") {
        dragFrames += 1;
        const displacement = Math.sin(phaseElapsed / 190) * 46;
        const positions = Object.fromEntries(
          ids.flatMap((id) => {
            const point = baseline[id];
            return point
              ? [[id, { x: point.x + displacement, y: point.y + displacement * 0.22 }]]
              : [];
          }),
        );
        store.apply({ type: "move", positions });
        if (phaseElapsed >= 2000) {
          store.apply({ type: "move", positions: baseline });
          phase = "pan";
          phaseStarted = now;
        }
        animationFrame.current = requestAnimationFrame(frame);
        return;
      }

      panFrames += 1;
      flow.setViewport({
        x: initialViewport.x + Math.sin(phaseElapsed / 330) * 16,
        y: initialViewport.y + Math.cos(phaseElapsed / 370) * 12,
        zoom: initialViewport.zoom * (1 + Math.sin(phaseElapsed / 410) * 0.025),
      });
      if (phaseElapsed < 2000) {
        animationFrame.current = requestAnimationFrame(frame);
        return;
      }
      void flow.setViewport(initialViewport);
      setBenchmark({
        kind: "done",
        dragFps: Math.round((dragFrames * 1000) / 2000),
        panFps: Math.round((panFrames * 1000) / phaseElapsed),
        slow,
      });
      animationFrame.current = undefined;
    };
    animationFrame.current = requestAnimationFrame(frame);
  }, [benchmark.kind, flow, state.items, state.selection, store]);

  const canvasBounds = canvasRef.current?.getBoundingClientRect();
  const draftPath = draft
    .map((point) => {
      const screen = flow.flowToScreenPosition(point);
      return `${screen.x - (canvasBounds?.left ?? 0)},${screen.y - (canvasBounds?.top ?? 0)}`;
    })
    .join(" ");

  return (
    <section className="spike-workbench">
      <div className="spike-toolbar" role="toolbar" aria-label={t("spike.toolbar.label")}>
        <button aria-pressed={!penActive} onClick={() => setPenActive(false)} type="button">
          {t("spike.toolbar.select")}
        </button>
        <button aria-pressed={penActive} onClick={() => setPenActive(true)} type="button">
          {t("spike.toolbar.pen")}
        </button>
        <span className="spike-toolbar-separator" />
        <button onClick={() => addShape("sticky")} type="button">
          {t("spike.toolbar.sticky")}
        </button>
        <button onClick={() => addShape("rectangle")} type="button">
          {t("spike.toolbar.rectangle")}
        </button>
        <button onClick={() => addShape("ellipse")} type="button">
          {t("spike.toolbar.ellipse")}
        </button>
        <span className="spike-toolbar-separator" />
        <button
          disabled={state.selection.length === 0}
          onClick={() =>
            store.apply({ type: "change-layer", ids: state.selection, direction: "backward" })
          }
          type="button"
        >
          {t("spike.toolbar.backward")}
        </button>
        <button
          disabled={state.selection.length === 0}
          onClick={() =>
            store.apply({ type: "change-layer", ids: state.selection, direction: "forward" })
          }
          type="button"
        >
          {t("spike.toolbar.forward")}
        </button>
        <span className="spike-toolbar-separator" />
        <button onClick={switchDataset} type="button">
          {state.dataset === "demo"
            ? t("spike.toolbar.loadPerformance")
            : t("spike.toolbar.loadDemo")}
        </button>
        <button disabled={benchmark.kind === "running"} onClick={runBenchmark} type="button">
          {benchmark.kind === "running" ? t("spike.benchmark.running") : t("spike.benchmark.run")}
        </button>
        <button onClick={exportPng} type="button">
          {t("spike.toolbar.exportPng")}
        </button>
        <button onClick={exportSvg} type="button">
          {t("spike.toolbar.exportSvg")}
        </button>
      </div>
      <div className="spike-metrics" aria-live="polite">
        <span>{t("spike.metrics.nodes", { count: nodes.length })}</span>
        <span>{t("spike.metrics.edges", { count: edges.length })}</span>
        <span>{t("spike.metrics.operations", { count: state.operationCount })}</span>
        {benchmark.kind === "done" && (
          <span
            className={
              Math.min(benchmark.dragFps, benchmark.panFps) >= 55 ? "spike-pass" : "spike-warn"
            }
          >
            {t("spike.benchmark.result", {
              drag: benchmark.dragFps,
              pan: benchmark.panFps,
              slow: benchmark.slow,
            })}
          </span>
        )}
        {exportMessage && <span>{exportMessage}</span>}
      </div>
      <div className="spike-canvas" ref={canvasRef}>
        <ReactFlow
          deleteKeyCode={["Backspace", "Delete"]}
          edgeTypes={edgeTypes}
          edges={edges}
          fitView
          fitViewOptions={{ padding: 0.16 }}
          minZoom={0.05}
          multiSelectionKeyCode={["Meta", "Control", "Shift"]}
          nodeTypes={nodeTypes}
          nodes={nodes}
          nodesConnectable={false}
          onlyRenderVisibleElements
          onNodesChange={(changes) => applyNodeChangesToStore(changes, state, store.apply)}
          panOnDrag={[1, 2]}
          selectionMode={SelectionMode.Partial}
          selectionOnDrag
        >
          <Background color="#cbd5e1" gap={24} size={1.2} variant={BackgroundVariant.Dots} />
          <Controls />
          <MiniMap pannable zoomable />
          <ViewportPortal>
            {state.presence.map((participant) => (
              <div
                className="spike-remote-cursor"
                key={participant.name}
                style={{
                  color: participant.color,
                  transform: `translate(${participant.cursor.x}px, ${participant.cursor.y}px)`,
                }}
              >
                <svg aria-hidden="true" height="28" viewBox="0 0 24 28" width="24">
                  <path
                    d="M2 1L21 18L12 19L8 27L2 1Z"
                    fill="currentColor"
                    stroke="white"
                    strokeWidth="2"
                  />
                </svg>
                <span style={{ backgroundColor: participant.color }}>{participant.name}</span>
              </div>
            ))}
          </ViewportPortal>
        </ReactFlow>
        {penActive && (
          <div
            aria-label={t("spike.penSurface")}
            className="spike-pen-surface"
            onPointerDown={onPenPointerDown}
            onPointerMove={onPenPointerMove}
            onPointerUp={onPenPointerUp}
            role="application"
          >
            {draft.length > 1 && (
              <svg aria-hidden="true" className="spike-pen-preview">
                <polyline fill="none" points={draftPath} stroke="#e11d48" strokeWidth="7" />
              </svg>
            )}
          </div>
        )}
      </div>
    </section>
  );
}

export function CanvasSpike() {
  const store = useMemo(() => new CanvasStore(createDemoState()), []);
  return (
    <ReactFlowProvider>
      <Canvas store={store} />
    </ReactFlowProvider>
  );
}
