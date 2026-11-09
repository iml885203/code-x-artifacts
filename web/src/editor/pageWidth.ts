// Whether the page is shown wide: the person's own choice, kept in their browser (it is a convenience, so a browser that
// refuses storage just starts narrow every time).
import { useState } from "react";

const key = "pictor.docWide";

function read(): boolean {
  try {
    return localStorage.getItem(key) === "1";
  } catch {
    return false;
  }
}

export function usePageWidth(): [boolean, () => void] {
  const [wide, setWide] = useState(read);
  return [
    wide,
    () => {
      const next = !wide;
      setWide(next);
      try {
        localStorage.setItem(key, next ? "1" : "0");
      } catch {
        // kept for this visit only
      }
    },
  ];
}
