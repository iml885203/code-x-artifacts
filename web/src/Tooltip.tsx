import * as Primitive from "@radix-ui/react-tooltip";
import { useState, type ReactElement, type ReactNode } from "react";
import "./Tooltip.css";

/** Non-interactive hints share positioning, descriptions and dismissal. Triggers keep their own accessible names. */
export function Tooltip({ content, children, disabled = false }: { content: ReactNode; children: ReactElement; disabled?: boolean }) {
  const [open, setOpen] = useState(false);
  return (
    <Primitive.Provider delayDuration={350} skipDelayDuration={100}>
      <Primitive.Root open={!disabled && open} onOpenChange={setOpen}>
        <Primitive.Trigger asChild>{children}</Primitive.Trigger>
        <Primitive.Portal>
          <Primitive.Content className="pictor-tooltip" sideOffset={6} collisionPadding={12}>
            {content}
          </Primitive.Content>
        </Primitive.Portal>
      </Primitive.Root>
    </Primitive.Provider>
  );
}
