import type { C4Item } from "./model";

function escapeXml(value: string) {
  return value
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&apos;");
}

export function serializeC4NodeSvg(item: C4Item) {
  const caption = escapeXml(item.caption.toUpperCase());
  const name = escapeXml(item.name);
  const technology = escapeXml(item.technology);
  const description = escapeXml(item.description);
  return `<svg xmlns="http://www.w3.org/2000/svg" width="320" height="190" viewBox="0 0 320 190">
  <rect x="2" y="2" width="316" height="186" rx="18" fill="#1168bd" stroke="#0b4f91" stroke-width="4"/>
  <text x="160" y="36" text-anchor="middle" font-family="system-ui, sans-serif" font-size="12" font-weight="700" fill="#dbeafe">${caption}</text>
  <text x="160" y="68" text-anchor="middle" font-family="system-ui, sans-serif" font-size="20" font-weight="700" fill="white">${name}</text>
  <text x="160" y="94" text-anchor="middle" font-family="system-ui, sans-serif" font-size="13" fill="#dbeafe">${technology}</text>
  <foreignObject x="24" y="108" width="272" height="60"><div xmlns="http://www.w3.org/1999/xhtml" style="font: 13px system-ui, sans-serif; color: white; text-align: center; line-height: 1.35">${description}</div></foreignObject>
</svg>`;
}

export function downloadText(filename: string, type: string, contents: string) {
  const url = URL.createObjectURL(new Blob([contents], { type }));
  const anchor = document.createElement("a");
  anchor.download = filename;
  anchor.href = url;
  anchor.click();
  URL.revokeObjectURL(url);
}
