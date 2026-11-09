import type { ReactNode } from "react";

function CanvasIcon({ children, className = "" }: { children: ReactNode; className?: string }) {
  return <svg className={`icon canvas-icon ${className}`} viewBox="0 0 24 24" aria-hidden="true">{children}</svg>;
}

export const Chevron = () => <CanvasIcon className="canvas-chevron"><path d="M6 9l6 6 6-6" /></CanvasIcon>;
export const Pointer = () => <CanvasIcon><path d="M5 3l14 8-6 2-2 6z" /></CanvasIcon>;
export const Hand = () => <CanvasIcon><path d="M8 12V6a1.5 1.5 0 013 0v5V4.5a1.5 1.5 0 013 0V11V6a1.5 1.5 0 013 0v9a6 6 0 01-6 6h-1a6 6 0 01-5-3l-3-5a1.5 1.5 0 012.5-1.5L8 14" /></CanvasIcon>;
export const TextIcon = () => <CanvasIcon><path d="M5 6V4h14v2M12 4v16M9 20h6" /></CanvasIcon>;
export const FrameIcon = () => <CanvasIcon><path d="M8 3v18M16 3v18M3 8h18M3 16h18" /></CanvasIcon>;
export const RectangleIcon = () => <CanvasIcon><rect x="4" y="6" width="16" height="12" rx="2" /></CanvasIcon>;
export const OvalIcon = () => <CanvasIcon><ellipse cx="12" cy="12" rx="8" ry="6" /></CanvasIcon>;
export const PlayIcon = () => <CanvasIcon><path d="M7 5l12 7-12 7z" /></CanvasIcon>;
export const PanelIcon = () => <CanvasIcon><rect x="3" y="4" width="18" height="16" rx="2" /><path d="M15 4v16" /></CanvasIcon>;
export const DownloadIcon = () => <CanvasIcon><path d="M12 4v11M7 11l5 5 5-5M5 20h14" /></CanvasIcon>;
export const PlusIcon = () => <CanvasIcon><path d="M12 5v14M5 12h14" /></CanvasIcon>;
export const SingleIcon = () => <CanvasIcon><rect x="5" y="5" width="14" height="14" rx="1" /></CanvasIcon>;
export const GridIcon = () => <CanvasIcon><rect x="4" y="4" width="6" height="6" /><rect x="14" y="4" width="6" height="6" /><rect x="4" y="14" width="6" height="6" /><rect x="14" y="14" width="6" height="6" /></CanvasIcon>;

export const PenIcon = () => <CanvasIcon><path d="M4 20l1-5L16 4l4 4L9 19z" /></CanvasIcon>;

export const NibIcon = () => <CanvasIcon><path d="m12 3 8 8-7 9-5-5-5-5zM3 21l5-6M12 3l-2 7 4 4 6-3" /><circle cx="12" cy="11" r="1" /></CanvasIcon>;
export const LineIcon = () => <CanvasIcon><path d="M4 20 20 4" /></CanvasIcon>;
export const ArrowIcon = () => <CanvasIcon><path d="M4 20 20 4M9 4h11v11" /></CanvasIcon>;
export const ExpandIcon = () => <CanvasIcon><path d="M8 3H3v5m13-5h5v5M3 16v5h5m13-5v5h-5" /></CanvasIcon>;

export const MoreIcon = () => <CanvasIcon><circle cx="5" cy="12" r="1" /><circle cx="12" cy="12" r="1" /><circle cx="19" cy="12" r="1" /></CanvasIcon>;
