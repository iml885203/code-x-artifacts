import { useEffect, useRef } from "react";
import { MarkMotion } from "./MarkMotion";
/** Interactive brand mark; pointer depth never moves the sign-in controls. */
export function LoginShowcase({ animate }: { animate: boolean }) {
  const root = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const scene = root.current;
    if (!scene || !animate || typeof matchMedia !== "function") return;
    const reduced = matchMedia("(prefers-reduced-motion: reduce)");
    const pointer = matchMedia("(hover: hover) and (pointer: fine) and (min-width: 900px)");
    const layers = [...scene.querySelectorAll<HTMLElement>(".signin-depth")];
    let frame = 0;
    let x = 0, y = 0;
    const reset = () => {
      cancelAnimationFrame(frame); frame = 0;
      layers.forEach(layer => { layer.getAnimations().forEach(animation => { animation.cancel(); }); });
    };
    const draw = () => {
      frame = 0;
      layers.forEach(layer => {
        const depth = 7;
        const from = getComputedStyle(layer).transform;
        layer.getAnimations().forEach(animation => { animation.cancel(); });
        layer.animate([{ transform: from }, { transform: `translate(${String(x * depth)}px, ${String(y * depth)}px)` }],
          { duration: 500, easing: "cubic-bezier(.16,1,.3,1)", fill: "forwards" });
      });
    };
    const move = (event: PointerEvent) => {
      if (reduced.matches || !pointer.matches || event.pointerType === "touch") return;
      const box = scene.getBoundingClientRect();
      x = Math.max(-1, Math.min(1, (event.clientX - box.left) / box.width * 2 - 1));
      y = Math.max(-1, Math.min(1, (event.clientY - box.top) / box.height * 2 - 1));
      if (!frame) frame = requestAnimationFrame(draw);
    };
    scene.addEventListener("pointermove", move);
    scene.addEventListener("pointerleave", reset);
    reduced.addEventListener("change", reset);
    pointer.addEventListener("change", reset);
    return () => {
      reset();
      scene.removeEventListener("pointermove", move);
      scene.removeEventListener("pointerleave", reset);
      reduced.removeEventListener("change", reset);
      pointer.removeEventListener("change", reset);
    };
  }, [animate]);
  return <div className="signin-showcase" ref={root}>
    <div className="signin-field-mark signin-depth"><MarkMotion animate={animate} /></div>
  </div>;
}
