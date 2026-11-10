import { useEffect, useRef } from "react";
import { useT } from "../i18n";
import { finalMark, markFormation } from "./markFormation";

/** Form once, then let CSS gently float the two planes. No frame loop after formation. */
export function MarkMotion({ animate }: { animate: boolean }) {
  const t = useT();
  const formationFrame = useRef(0);
  const bursts = useRef<(SVGGElement | null)[]>([]);
  const svg = useRef<SVGSVGElement>(null);
  const elapsed = useRef(0);
  const first = useRef<SVGPathElement>(null);
  const second = useRef<SVGPathElement>(null);
  useEffect(() => {
    if (typeof matchMedia !== "function") return;
    const paths = [first.current, second.current];
    const groups = [...bursts.current];
    const reduced = matchMedia("(prefers-reduced-motion: reduce)");
    let previous = performance.now();
    const draw = (progress: number) => {
      markFormation(progress).forEach((d, i) => paths[i]?.setAttribute("d", d));
    };
    const tick = (now: number) => {
      elapsed.current = Math.min(3200, elapsed.current + now - previous);
      previous = now;
      draw(elapsed.current / 3200);
      if (elapsed.current < 3200) formationFrame.current = requestAnimationFrame(tick);
      else svg.current?.classList.add("is-formed");
    };
    const sync = () => {
      cancelAnimationFrame(formationFrame.current);
      svg.current?.classList.toggle("is-hidden", document.hidden);
      if (document.hidden || reduced.matches) {
        groups.forEach(group => { group?.getAnimations().forEach(animation => { animation.cancel(); }); });
      }
      if (!animate || reduced.matches) {
        elapsed.current = 3200;
        draw(1);
        svg.current?.classList.toggle("is-formed", animate);
      } else if (!document.hidden) {
        previous = performance.now();
        if (elapsed.current < 3200) formationFrame.current = requestAnimationFrame(tick);
        else { draw(1); svg.current?.classList.add("is-formed"); }
      }
    };
    sync();
    const preference = sync;
    const visibility = sync;
    reduced.addEventListener("change", preference);
    document.addEventListener("visibilitychange", visibility);
    return () => {
      cancelAnimationFrame(formationFrame.current);
      groups.forEach(group => { group?.getAnimations().forEach(animation => { animation.cancel(); }); });
      reduced.removeEventListener("change", preference);
      document.removeEventListener("visibilitychange", visibility);
    };
  }, [animate]);
  const burst = () => {
    const reduced = typeof matchMedia !== "function" || matchMedia("(prefers-reduced-motion: reduce)").matches;
    cancelAnimationFrame(formationFrame.current);
    elapsed.current = 3200;
    [first.current, second.current].forEach((path, i) => { path?.setAttribute("d", finalMark[i] ?? ""); });
    if (animate) svg.current?.classList.add("is-formed");
    bursts.current.forEach((group, i) => {
      if (!group || typeof group.animate !== "function") return;
      const from = getComputedStyle(group).transform;
      group.getAnimations().forEach(animation => { animation.cancel(); });
      if (reduced) {
        group.animate([{ opacity: .65 }, { opacity: 1 }], { duration: 160 });
        return;
      }
      const direction = i === 0 ? -1 : 1;
      group.animate([
        { transform: from, offset: 0 },
        { transform: `translate(${String(direction * 38)}px, ${String(direction * 20)}px) rotate(${String(direction * 14)}deg)`, offset: .28, easing: "cubic-bezier(.2,.8,.2,1)" },
        { transform: `translate(${String(-direction * 5)}px, ${String(-direction * 3)}px) rotate(${String(-direction * 2)}deg)`, offset: .72, easing: "cubic-bezier(.16,1,.3,1)" },
        { transform: "translate(0,0) rotate(0)", offset: 1 },
      ], { duration: 900, easing: "cubic-bezier(.16,1,.3,1)" });
    });
  };
  return <button className="signin-mark-button" type="button" aria-label={t("login.mark.interact")} onClick={burst}>
    <svg ref={svg} viewBox="0 0 256 256" aria-hidden="true" focusable="false">
      <g className="mark-burst" ref={node => { bursts.current[0] = node; }}><path className="mark-plane mark-plane-a" ref={first} d={finalMark[0]} fillRule="evenodd" /></g>
      <g className="mark-burst" ref={node => { bursts.current[1] = node; }}><path className="mark-plane mark-plane-b" ref={second} d={finalMark[1]} /></g>
    </svg>
  </button>;
}
