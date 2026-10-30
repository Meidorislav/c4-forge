import { describe, expect, it } from "vitest";
import { serializeC4NodeSvg } from "./export";
import type { C4Item } from "./model";

describe("C4 node SVG sketch", () => {
  it("serializes one node type without allowing XML injection", () => {
    const item: C4Item = {
      id: "node",
      kind: "c4",
      shape: "box",
      caption: "Container",
      name: "API <gateway>",
      technology: "Go & PostgreSQL",
      description: 'Routes "trusted" requests',
      position: { x: 0, y: 0 },
      zIndex: 20,
    };
    const svg = serializeC4NodeSvg(item);
    expect(svg).toContain("API &lt;gateway&gt;");
    expect(svg).toContain("Go &amp; PostgreSQL");
    expect(svg).toContain("Routes &quot;trusted&quot; requests");
    expect(svg).not.toContain("API <gateway>");
  });
});
