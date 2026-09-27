# ADR-0001: Canvas library for C4 diagrams and freeform drawing

- **Status:** Proposed — pending the spike described below
- **Date:** 2026-09-28
- **Related requirements:** DG-4…DG-18, FR-1…FR-6, CO-1, CO-2, EX-1, NF-3, NF-16

## Context

The canvas is the core of the product. The same canvas has to render two kinds of content:

1. **Model-backed C4 content.** Elements with rich content (type caption, name, technology, description, icon) and several shapes (box, person, cylinder, queue, web browser, mobile). Relationships are *derived* from the model, not drawn directly: a relationship appears when both ends are on the diagram, and implied relationships are rolled up from nested elements (DG-7, DG-8). The canvas also shows scope boundaries (DG-10) and drill-down (DG-11). Later it needs routing styles and waypoints (DG-14) and auto-layout (DG-13).
2. **Freeform content.** Sticky notes, rectangles, ellipses and text in MVP (FR-1). In v1: arrows that bind to shapes and elements, freehand pen, images, frames (FR-2, FR-3).

The canvas must also support:

- Real-time collaboration with remote cursors and selections (CO-1, CO-2). The source of truth is our own model and sync engine (ADR-0002), not the canvas library's internal state.
- Undo/redo that is aware of collaborative editing (DG-18). This must work on our operations, not on the library's history.
- Multi-select, alignment, snapping, copy/paste (DG-17).
- Export to PNG in MVP and SVG in v1 (EX-1).
- Around 300 elements and shapes per diagram without noticeable lag (section 4 of the requirements).

The constraints come from the requirements:

- **NF-16.** The project is Apache-2.0. Dependencies must not require a paid license, a license key or a watermark in production, and must not carry copyleft that hinders self-hosting.
- **NF-3.** No network calls at runtime, so no license servers and no CDNs.
- The frontend is React (a decision made earlier).

## Options considered

### A. tldraw SDK

A very capable infinite-canvas SDK: custom shapes, arrow bindings, freehand drawing, frames, images, undo, and a mature interaction model.

**Rejected on licensing.** Since SDK 4.0 the default license allows only development use. Production requires a license key. The options are:

- a 100-day trial;
- a free hobby license, non-commercial only, with a mandatory "made with tldraw" watermark;
- a paid commercial license.

An open-source project may depend on it, but every downstream deployment would need its own license. This violates NF-16. Its official multiplayer sync is also a TypeScript server component, which does not fit a Go backend.

### B. Excalidraw (`@excalidraw/excalidraw`)

MIT-licensed, with an excellent hand-drawn freeform experience and existing collaboration support.

**Rejected on extensibility.** There is still no supported public API for custom element types (long-open issues, see references). C4 elements with structured content would have to be faked with groups of primitive elements or implemented in a fork. Derived and implied relationships, scope boundaries and drill-down go against its flat scene model. Its hand-drawn look is also not what most users expect from architecture diagrams.

### C. React Flow (`@xyflow/react`) + our own freeform layer

MIT-licensed. React Flow is a mature library for node-and-edge editors. It already provides:

- nodes and edges rendered as React components, so rich HTML content is easy;
- a pan/zoom viewport, selection (including box selection), handles and connection gestures;
- minimap and controls;
- a controlled mode where the application owns the state.

Freeform content fits the same model:

- sticky notes, rectangles, ellipses, text and images become custom node types;
- freehand strokes become nodes that render an SVG path (e.g. via `perfect-freehand`, MIT);
- bound arrows become edges between freeform nodes and/or C4 nodes.

The xyflow team publishes whiteboard examples: rectangle drawing, lasso selection and eraser are free, while freehand drawing is a paid "Pro" example. These are reference code only, not a dependency. We would implement freehand drawing ourselves.

What we would have to build ourselves:

- snapping guides and alignment;
- orthogonal routing with waypoints (DG-14);
- auto-layout (DG-13; library choice is a separate decision);
- SVG export — DOM-based nodes need our own SVG serializer, while PNG works via DOM-to-image;
- undo/redo — needed on our operations anyway.

Risks:

- DOM rendering scales worse than canvas or WebGL. Around 300 nodes is within React Flow's usual comfort zone, but we must verify it with our node components.
- A whiteboard feel (smooth freehand, many small shapes) is not React Flow's primary use case.

### D. maxGraph

Apache-2.0 TypeScript fork of mxGraph. Strong diagramming features: stencils and custom shapes, orthogonal/Manhattan edge routing, SVG rendering, an undo manager.

**Not chosen.**

- It is imperative and framework-agnostic. Its own graph model would have to be kept in sync with our model and sync engine in both directions.
- Rich HTML content inside shapes is awkward compared to React components.
- Freeform/whiteboard interactions are limited.
- The project still lists behaviour gaps compared to mxGraph.

It remains a useful reference for edge routing algorithms.

### E. JointJS (core)

MPL-2.0. File-level copyleft is acceptable as a dependency. SVG-based, good for diagram editors.

**Not chosen.** Several features we need are in the commercial JointJS+ tier: selection, undo/redo, minimap, stencil and scroller. Implementing them ourselves removes most of the advantage over React Flow. It also has no freeform drawing model.

### F. Our own renderer (SVG, or canvas via Konva/PixiJS — both MIT)

**Not chosen for now.** It gives full control and the best performance ceiling. But we would build viewport, hit testing, selection, text editing, accessibility and interaction details from scratch. That is months of work before any product value, which contradicts incremental delivery (NF-18).

## Decision (proposed)

Use **React Flow (`@xyflow/react`) as the canvas view layer**, with C4 elements and freeform shapes implemented as our own custom node and edge types on a single canvas.

Architectural rules that come with this choice:

1. **React Flow is a view, not a store.** It runs in controlled mode. Our client store, fed by the sync engine, is the source of truth. React Flow state is derived from it, and user gestures are translated into model operations.
2. **Canvas adapter boundary.** Model, operations, derived views (visible relationships, implied relationships, boundaries), undo/redo and import/export must not import React Flow. Only the canvas adapter does. This keeps option F (own renderer) open if we outgrow React Flow.
3. **One canvas, two layers.** Freeform shapes are separate node types with their own z-order band (below or above C4 content, per shape). Freeform shapes never create model entities. The only exception is the explicit FR-5 conversion.

The decision becomes **Accepted** only after the spike below passes.

## Spike (validation)

A throwaway prototype on a separate branch, never merged. It must show:

| # | Check | Pass criterion |
| --- | --- | --- |
| 1 | Performance | 300 C4 nodes + 400 edges: pan/zoom and dragging a multi-selection stay smooth (~60 fps on a mid-range laptop). |
| 2 | C4 node | A custom node with caption/name/technology/description and person/cylinder shapes; the node size follows its content. |
| 3 | Derived edges | Floating edges between node borders with labels; parallel edges between the same pair do not overlap. |
| 4 | Freeform basics | Sticky note with inline text editing; rectangle/ellipse; freehand stroke with `perfect-freehand`. |
| 5 | Bound arrow | A freeform arrow bound to a sticky note and to a C4 node follows both while dragging. |
| 6 | Layers | Freeform shapes can sit above or below C4 nodes; selection works across both. |
| 7 | Controlled mode | All changes flow through an external store as operations, with no lost frames while dragging. |
| 8 | Presence | Remote cursors and selection highlights rendered in flow coordinates. |
| 9 | Export | PNG export of the whole diagram; a sketch of SVG export for one node type. |

If checks 1, 4 or 5 fail in a way that cannot be fixed within React Flow, we go back to this ADR. The most likely fallback is option F with an SVG renderer.

## Consequences

**Positive**

- Permissive license with no key or watermark, which fits NF-16 and NF-3.
- Fast path to a usable MVP: viewport, selection, connections and minimap come ready-made.
- C4 nodes are plain React components, so rich content, icons, theming and i18n are straightforward.
- A large ecosystem and community, so hiring and contributing are easier.

**Negative**

- We own several non-trivial features: freehand drawing, snapping, orthogonal routing, SVG export, collaborative undo.
- DOM rendering puts a practical ceiling on diagram size. That is acceptable for our targets and mitigated by the adapter boundary.
- The whiteboard experience depends on our own implementation quality rather than a specialised whiteboard engine.

**Follow-ups**

- ADR-0002: collaborative editing engine — defines the operations the canvas adapter emits.
- A separate decision on the auto-layout library (candidates to evaluate include dagre and elkjs; license compatibility must be checked).

## References

- tldraw license: https://tldraw.dev/community/license, https://tldraw.dev/sdk-features/license-key
- Excalidraw custom elements: https://github.com/excalidraw/excalidraw/issues/4957, https://github.com/excalidraw/excalidraw/issues/8184
- React Flow whiteboard examples: https://reactflow.dev/learn/advanced-use/whiteboard
- maxGraph: https://github.com/maxGraph/maxGraph
- JointJS licensing: https://www.jointjs.com/license
