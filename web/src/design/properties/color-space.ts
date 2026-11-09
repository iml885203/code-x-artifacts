import type { Rgba } from "./css";
export interface Hsv { h: number; s: number; v: number }
export function toHsv({r,g,b}: Rgba): Hsv {
  const max = Math.max(r,g,b), min = Math.min(r,g,b), d = max-min;
  const hue = d === 0 ? 0 : max === r ? ((g-b)/d)%6 : max === g ? (b-r)/d+2 : (r-g)/d+4;
  return {h:(hue*60+360)%360,s:max === 0 ? 0 : d/max,v:max/255};
}
export function fromHsv({h,s,v}: Hsv, a: number): Rgba {
  const c=v*s, x=c*(1-Math.abs((h/60)%2-1)), m=v-c;
  const [r,g,b] = h < 60 ? [c,x,0] : h < 120 ? [x,c,0] : h < 180 ? [0,c,x] : h < 240 ? [0,x,c] : h < 300 ? [x,0,c] : [c,0,x];
  return {r:Math.round((r+m)*255),g:Math.round((g+m)*255),b:Math.round((b+m)*255),a};
}
