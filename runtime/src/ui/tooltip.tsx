import * as React from "react";
import * as T from "@radix-ui/react-tooltip";
import { cn } from "../lib/utils";
export const TooltipProvider = T.Provider;
export const Tooltip = T.Root;
export const TooltipTrigger = T.Trigger;
export const TooltipContent = ({ className, sideOffset = 4, ...p }: React.ComponentProps<typeof T.Content>) => (
  <T.Portal><T.Content sideOffset={sideOffset} className={cn("z-50 rounded-md bg-foreground px-3 py-1.5 text-xs text-background", className)} {...p} /></T.Portal>
);
