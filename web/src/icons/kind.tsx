import { Icon } from "./Icon";

/** The general artifact (html and react alike): code brackets with a slash. */
export const CodeIcon = () => (
  <Icon>
    <path d="m5 4.5-3.5 3.5L5 11.5M11 4.5 14.5 8 11 11.5M9.3 3l-2.6 10" />
  </Icon>
);
export const FileText = () => (
  <Icon>
    <path d="M3.5 1.5h6l3 3v10h-9z" />
    <path d="M9.5 1.5v3h3M5.5 8h5M5.5 10.5h5" />
  </Icon>
);
export const Palette = () => (
  <Icon>
    <path d="M8 2a6 6 0 1 0 0 12c1 0 1.5-.7 1.5-1.4 0-.9-.7-1.2-.7-2 0-.8.6-1.1 1.4-1.1H12a2.5 2.5 0 0 0 2.5-2.5C14.5 4.4 11.6 2 8 2z" />
    <path d="M5 7.5h.01M7 5h.01M10 5.2h.01" />
  </Icon>
);
/** Stacked pages, for slides. */
export const Layers = () => (
  <Icon>
    <path d="M5.5 2.5h8v7h-8z" />
    <path d="M2.5 5.5v7h8" />
  </Icon>
);
/** A screen on a stand, for slides. */
export const Screen = () => (
  <Icon>
    <path d="M2.5 2.5h11v7.5h-11z" />
    <path d="M8 10v3.5M5.5 13.5h5" />
  </Icon>
);
/** A pencil, for a design. */
export const Brush = () => (
  <Icon>
    <path d="m10.5 2 3.5 3.5L6 13.5 2 14l.5-4z" />
    <path d="m9 3.5 3.5 3.5" />
  </Icon>
);
/** Two shapes joined by an arrow, for a board. */
export const Board = () => (
  <Icon>
    <path d="M1.5 3h5v4h-5z" />
    <circle cx="11.5" cy="11.5" r="2.5" />
    <path d="M6.5 5h3.5v4" />
    <path d="m8.5 7.5 1.5 1.5 1.5-1.5" />
  </Icon>
);
/** Bars, for a dashboard. */
export const BarChart = () => (
  <Icon>
    <path d="M3 13.5v-5M8 13.5v-11M13 13.5v-8" />
  </Icon>
);
/** A play triangle in a circle, for an animation. */
export const PlayCircle = () => (
  <Icon>
    <circle cx="8" cy="8" r="6" />
    <path d="m6.7 5.5 3.6 2.5-3.6 2.5z" />
  </Icon>
);
/** A square, a circle and a triangle, for a design system. */
export const Shapes = () => (
  <Icon>
    <path d="M2 2h5v5H2z" />
    <circle cx="11.5" cy="4.5" r="2.5" />
    <path d="M5 9.5 8 14.5H2z" />
    <path d="M10 10h4v4h-4z" />
  </Icon>
);
