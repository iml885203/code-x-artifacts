import { Icon } from "./Icon";

export const Pin = () => (
  <Icon>
    <path d="M10 2.5 13.5 6l-1.5.8-2 2.4.2 2.3-1 1L4.8 8l1-1 2.3.2L10.5 5z" />
    <path d="M5.5 10.5 2.5 13.5" />
  </Icon>
);
/** The pin of a pinned artifact: `.filled` is what turns the fill on, because svg.icon sets fill to none. */
export const PinFilled = () => (
  <svg className="icon filled" viewBox="0 0 16 16" aria-hidden="true">
    <path d="M10 2.5 13.5 6l-1.5.8-2 2.4.2 2.3-1 1L4.8 8l1-1 2.3.2L10.5 5z" />
    <path d="M5.5 10.5 2.5 13.5" />
  </svg>
);
