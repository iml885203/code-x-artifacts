import { useState } from "react";

/**
 * The card's preview: the artifact's own preview address in an inset, clipped screen on the kind's wash. The page
 * is invisible until it has loaded, so an empty or slow preview shows the wash and never a white flash or a made-up
 * placeholder.
 */
export function Preview({ id }: { id: string }) {
  const [loaded, setLoaded] = useState(false);
  return (
    <span className="thumb" data-thumb="">
      <span className={loaded ? "thumb-screen loaded" : "thumb-screen"}>
        {/* The preview address serves the page without the bridge, and the page's script runs in an opaque origin (allow-scripts alone) so a page drawn by script shows. Frames off screen are not loaded. */}
        <iframe
          src={`/raw/${id}?preview`}
          sandbox="allow-scripts"
          loading="lazy"
          tabIndex={-1}
          aria-hidden="true"
          title=""
          onLoad={() => {
            setLoaded(true);
          }}
        />
      </span>
    </span>
  );
}
