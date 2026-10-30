import { describe, expect, it, vi } from "vitest";
import { createDemoState, createPerformanceState } from "./model";
import { CanvasStore } from "./store";

describe("canvas operation store", () => {
  it("applies gestures as external operations and notifies subscribers", () => {
    const store = new CanvasStore(createDemoState());
    const listener = vi.fn();
    store.subscribe(listener);

    store.apply({
      type: "move",
      positions: { "system-forge": { x: 700, y: 210 } },
    });
    store.apply({ type: "select", ids: ["system-forge", "sticky-question"] });
    store.apply({
      type: "update-text",
      id: "sticky-question",
      text: "Edited through an operation",
    });

    const snapshot = store.getSnapshot();
    expect(snapshot.items["system-forge"]?.position).toEqual({ x: 700, y: 210 });
    expect(snapshot.items["sticky-question"]).toMatchObject({
      text: "Edited through an operation",
    });
    expect(snapshot.selection).toEqual(["system-forge", "sticky-question"]);
    expect(snapshot.operationCount).toBe(3);
    expect(listener).toHaveBeenCalledTimes(3);
  });

  it("creates the exact performance workload from ADR-0001", () => {
    const state = createPerformanceState();
    expect(state.order).toHaveLength(300);
    expect(state.relationships).toHaveLength(400);
    expect(state.selection).toHaveLength(20);
  });

  it("moves selected content between z-order bands", () => {
    const store = new CanvasStore(createDemoState());
    const before = store.getSnapshot().items["freeform-rectangle"]?.zIndex;
    store.apply({
      type: "change-layer",
      ids: ["freeform-rectangle"],
      direction: "forward",
    });
    expect(store.getSnapshot().items["freeform-rectangle"]?.zIndex).toBe((before ?? 0) + 10);
  });
});
