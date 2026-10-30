import { getStroke } from "perfect-freehand";
import type { Point } from "./model";

function average(first: readonly number[], second: readonly number[]) {
  return [
    ((first[0] ?? 0) + (second[0] ?? 0)) / 2,
    ((first[1] ?? 0) + (second[1] ?? 0)) / 2,
  ] as const;
}

export function strokePath(points: readonly Point[]) {
  const outline = getStroke(
    points.map((point) => [point.x, point.y]),
    { size: 7, thinning: 0.55, smoothing: 0.65, streamline: 0.5 },
  );
  if (outline.length < 3) return "";

  const first = outline[0];
  const second = outline[1];
  const last = outline[outline.length - 1];
  if (!first || !second || !last) return "";

  let path = `M ${first[0]} ${first[1]} Q ${second[0]} ${second[1]} `;
  for (let index = 2; index < outline.length; index += 1) {
    const current = outline[index];
    const next = outline[index + 1];
    if (!current) continue;
    const midpoint = next ? average(current, next) : current;
    path += `${midpoint[0]} ${midpoint[1]} `;
  }
  return `${path}Z`;
}
