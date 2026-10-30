// English UI strings. This catalog is the source of truth for translation
// keys: other languages must provide the same keys.
export const en = {
  app: {
    name: "c4-forge",
    tagline: "Model your software architecture with C4, together.",
  },
  status: {
    connecting: "Connecting to the server…",
    version: "Server version {{version}}",
    unavailable: "The server is unavailable.",
  },
  spike: {
    eyebrow: "M1 / 1.1 · Throwaway validation branch",
    checks: {
      title: "ADR-0001 checks",
      performance: "300 C4 nodes + 400 edges",
      c4Node: "Rich, content-sized C4 nodes",
      derivedEdges: "Floating and parallel derived edges",
      freeform: "Sticky, rectangle, ellipse and freehand",
      boundArrow: "Bound freeform arrow",
      layers: "Mixed selection and z-order",
      controlled: "External operation store",
      presence: "Remote cursor and selection",
      export: "Whole-diagram PNG and node SVG",
    },
    help: "Drag nodes, box-select across layers, edit shape text, or load the performance dataset. The operation counter proves that canvas gestures pass through the external store.",
    toolbar: {
      label: "Canvas spike tools",
      select: "Select",
      pen: "Pen",
      sticky: "+ Sticky",
      rectangle: "+ Rectangle",
      ellipse: "+ Ellipse",
      backward: "Send backward",
      forward: "Bring forward",
      loadPerformance: "Load 300 + 400",
      loadDemo: "Load demo",
      exportPng: "Export PNG",
      exportSvg: "Export node SVG",
    },
    metrics: {
      nodes: "{{count}} nodes",
      edges: "{{count}} edges",
      operations: "{{count}} operations",
    },
    benchmark: {
      run: "Run 4 s benchmark",
      running: "Benchmarking…",
      result: "{{drag}} drag fps · {{pan}} pan fps · {{slow}} slow frames",
    },
    export: {
      rendering: "Rendering export…",
      ready: "PNG downloaded",
      failed: "PNG export failed",
      svgReady: "SVG downloaded",
    },
    newShape: {
      sticky: "New sticky note",
      rectangle: "New rectangle",
      ellipse: "New ellipse",
    },
    penSurface: "Freehand drawing surface",
  },
} as const;
