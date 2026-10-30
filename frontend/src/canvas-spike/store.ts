import { useSyncExternalStore } from "react";
import { applyCanvasOperation, type CanvasOperation, type CanvasState } from "./model";

export class CanvasStore {
  private state: CanvasState;
  private readonly listeners = new Set<() => void>();

  constructor(initialState: CanvasState) {
    this.state = initialState;
  }

  getSnapshot = () => this.state;

  subscribe = (listener: () => void) => {
    this.listeners.add(listener);
    return () => this.listeners.delete(listener);
  };

  apply = (operation: CanvasOperation) => {
    const next = applyCanvasOperation(this.state, operation);
    if (next === this.state) return;
    this.state = next;
    for (const listener of this.listeners) listener();
  };
}

export function useCanvasStore(store: CanvasStore) {
  return useSyncExternalStore(store.subscribe, store.getSnapshot, store.getSnapshot);
}
