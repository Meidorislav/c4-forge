# React Flow canvas spike

- **Roadmap step:** M1 / 1.1
- **Date:** 2026-10-30
- **Branch:** `feat/canvas-spike-react-flow` (throwaway; never merge)
- **Libraries:** `@xyflow/react` 12.12.0, `perfect-freehand` 1.2.3,
  `html-to-image` 1.11.13 (all MIT)
- **Result:** pass

This prototype validates the nine checks in ADR-0001. It is deliberately a self-contained
workbench rather than production editor code. The branch is evidence for the decision and must not
be merged.

## Results

| # | Check | Result | Evidence |
| --- | --- | --- | --- |
| 1 | Performance | Pass, with a constraint | The production build loaded exactly 300 rich C4 nodes and 400 edges. At a practical working zoom with off-screen elements culled, a four-second benchmark measured 61 fps while dragging 20 selected nodes and 60 fps while panning and zooming, with zero frames over 25 ms. Fitting all content into the viewport measured 46–47 fps; see the constraint below. |
| 2 | C4 node | Pass | Custom React nodes render caption, name, technology and description. The demo includes box, person and cylinder variants. Height is measured from rendered content. |
| 3 | Derived edges | Pass | Relationships select handles on the facing node borders. Two relationships between the same pair use separate quadratic curves and labels. |
| 4 | Freeform basics | Pass | Sticky notes support inline text editing. Rectangle and ellipse nodes share the canvas with C4 content. The pen stores points and renders a pressure-smoothed SVG outline through `perfect-freehand`. |
| 5 | Bound arrow | Pass | A dashed freeform arrow binds a sticky note to a C4 node. Moving either endpoint updates the arrow during the drag. |
| 6 | Layers | Pass | Freeform and C4 nodes use separate z-index bands. Box selection selected six mixed C4/freeform items, and the forward/backward actions changed their bands. |
| 7 | Controlled mode | Pass | `CanvasStore` is external to React Flow. Move, select, edit, add, delete and layer gestures are model operations. A browser drag produced a sequence of operations with no dropped final position. Unit tests cover the store. |
| 8 | Presence | Pass | A remote cursor is rendered in flow coordinates through `ViewportPortal`; remote selections add colored rings to C4 and freeform nodes. |
| 9 | Export | Pass | PNG export calculates the bounds of the complete diagram and renders the transformed viewport with `html-to-image`. The spike also serializes one C4 box node to standalone SVG, with XML escaping covered by a unit test. |

## Performance constraint

React Flow remains suitable, but the production adapter must preserve object identity for unchanged
nodes and edges and enable visible-element culling. The first controlled implementation recreated
all 700 objects on every drag frame and measured 9 fps. Caching unchanged adapter objects removed
that bottleneck.

At `fitView`, all 300 rich DOM nodes and 400 SVG edges are visible simultaneously but too small to
read; this measured 46–47 fps in the spike browser. At a normal editing zoom, React Flow culls
off-screen elements and met the approximately 60 fps criterion. A future overview mode should use
level-of-detail rendering if it needs smooth interaction while every large node is visible.

## Reproduce

```bash
git fetch origin
git switch feat/canvas-spike-react-flow
cd frontend
pnpm install
pnpm test
pnpm build
pnpm exec vite preview
```

Open the preview, select **Load 300 + 400**, then **Run 4 s benchmark**. The benchmark has two
phases of two seconds: moving the 20 selected nodes through the external operation store, then
panning/zooming the viewport. Use the demo dataset for the remaining checks.

The automated checks are:

```bash
cd frontend
pnpm lint
pnpm typecheck
pnpm test
pnpm build
```

## Decision

Accept ADR-0001. React Flow meets the interaction, extensibility and practical performance needs of
the planned editor when used behind the adapter boundary described by the ADR. The performance
constraint above is an implementation requirement for the production adapter, not a reason to
change the library decision.
