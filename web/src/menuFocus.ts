/**
 * For a menu's `onCloseAutoFocus`: Radix gives focus back to the button that opened the menu a task after the
 * menu has left the page, which on a menu that fades out is a moment after the choice. A person who has already
 * clicked into the editor by then would have their typing go to the button, so focus stays where it is once it
 * is on something other than the menu being closed.
 */
export function keepFocusWhereItWent(event: Event): void {
  const active = document.activeElement;
  const menu = event.target;
  if (active !== null && active !== document.body && !(menu instanceof Node && menu.contains(active))) event.preventDefault();
}

/** For a menu's `onInteractOutside`: a closing menu still listens outside during its exit animation, so reopening it from its trigger must not dismiss it again. */
export function keepTriggerToggle(event: Event, trigger: HTMLElement | null): void {
  if (event.target instanceof Node && trigger?.contains(event.target)) event.preventDefault();
}
