/** Geometry and timing from the owner-approved offset-field motion study. */
type Point = readonly [number, number];
function at<T>(items: readonly T[], index: number): T {
  const value = items[index];
  if (value === undefined) throw new Error("invalid mark geometry index");
  return value;
}
const clamp = (x: number) => Math.max(0, Math.min(1, x));
const lerp = (a: number, b: number, t: number) => a + (b - a) * t;
const out = (x: number) => 1 - Math.pow(1 - clamp(x), 4);
const into = (x: number) => Math.pow(clamp(x), 3);
const interval = (t: number, a: number, b: number, curve = out) => curve((t - a) / (b - a));
const polygon = (points: readonly Point[]) => `M${points.map(([x, y]) => `${String(x)},${String(y)}`).join("L")}Z`;
const outline: Point[][] = [
  [[3, 3], [15, 3], [15, 9], [9, 15], [3, 15]],
  [[16, 10], [22, 10], [22, 22], [10, 22], [10, 16]],
];
const finalPolygons = outline.map(points => points.map(([x, y]): Point => [x * 256 / 24, y * 256 / 24]));
export const finalMark = finalPolygons.map(polygon);

function sample(points: readonly Point[], count = 96): Point[] {
  const lengths = points.map((p, i) => {
    const next = at(points, (i + 1) % points.length);
    return Math.hypot(next[0] - p[0], next[1] - p[1]);
  });
  const total = lengths.reduce((a, b) => a + b, 0);
  const exact = lengths.map(length => length / total * count);
  const counts = exact.map(value => Math.max(1, Math.floor(value)));
  let remaining = count - counts.reduce((a, b) => a + b, 0);
  const order = exact.map((value, i) => ({ i, fraction: value - Math.floor(value) })).sort((a, b) => b.fraction - a.fraction);
  for (let i = 0; remaining > 0; i++, remaining--) {
    const index = at(order, i % order.length).i;
    counts[index] = at(counts, index) + 1;
  }
  return points.flatMap((a, i) => Array.from({ length: at(counts, i) }, (_, j): Point => {
    const b = at(points, (i + 1) % points.length), q = j / at(counts, i);
    return [lerp(a[0], b[0], q), lerp(a[1], b[1], q)];
  }));
}
const initial = [0, 1].map(side => {
  const start = side ? -Math.PI / 2 : Math.PI / 2;
  return sample(Array.from({ length: 49 }, (_, i): Point => [
    128 + 64 * Math.cos(start + i / 48 * Math.PI),
    128 + 64 * Math.sin(start + i / 48 * Math.PI),
  ]));
});
const finals = finalPolygons.map(points => sample(points));
const circle = (r: number) => `M${String(128 - r)},128a${String(r)},${String(r)} 0 1,0 ${String(2 * r)},0a${String(r)},${String(r)} 0 1,0 ${String(-2 * r)},0Z`;

export function markFormation(t: number): readonly string[] {
  if (t >= .93) return finalMark;
  if (t < .42) {
    let radius: number, inner = 0;
    if (t < .09) radius = lerp(0, 7, interval(t, 0, .06));
    else if (t < .14) radius = lerp(7, 4, interval(t, .09, .14, into));
    else if (t < .28) {
      const p = interval(t, .14, .28);
      radius = lerp(4, 66, p);
      inner = Math.max(0, radius - lerp(4, 5, p));
    } else if (t < .35) {
      const p = interval(t, .28, .35, into);
      radius = lerp(66, 64, p); inner = lerp(61, 0, p);
    } else radius = 64;
    return [radius > 0 ? circle(radius) + (inner > 0 ? circle(inner) : "") : "", ""];
  }
  const spread = interval(t, .42, .52), join = interval(t, .59, .81), reshape = interval(t, .54, .79);
  const settle = t > .81 ? Math.sin(clamp((t - .81) / .12) * Math.PI) * 1.4 : 0;
  return finals.map((end, i) => {
    const direction = i ? 1 : -1;
    const distance = 22 * spread * (1 - join) + settle;
    const angle = direction * .12 * spread * (1 - join);
    return polygon(end.map((p, j): Point => {
      const x = lerp(at(at(initial, i), j)[0], p[0], reshape) - 128;
      const y = lerp(at(at(initial, i), j)[1], p[1], reshape) - 128;
      return [128 + x * Math.cos(angle) - y * Math.sin(angle) + direction * distance,
        128 + x * Math.sin(angle) + y * Math.cos(angle) + direction * distance * .25];
    }));
  });
}
