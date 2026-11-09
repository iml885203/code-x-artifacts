import * as React from "react";
import * as D from "@radix-ui/react-dialog";
import { cn } from "../lib/utils";
export const Dialog = D.Root;
export const DialogTrigger = D.Trigger;
export const DialogClose = D.Close;
export const DialogTitle = ({ className, ...p }: React.ComponentProps<typeof D.Title>) => <D.Title className={cn("text-lg font-semibold", className)} {...p} />;
export const DialogDescription = ({ className, ...p }: React.ComponentProps<typeof D.Description>) => <D.Description className={cn("text-sm text-muted-foreground", className)} {...p} />;
export const DialogContent = ({ className, children, ...p }: React.ComponentProps<typeof D.Content>) => (
  <D.Portal>
    <D.Overlay className="fixed inset-0 z-50 bg-black/60" />
    <D.Content className={cn("fixed left-1/2 top-1/2 z-50 grid w-full max-w-md -translate-x-1/2 -translate-y-1/2 gap-3 rounded-lg border border-border bg-card p-6", className)} {...p}>{children}</D.Content>
  </D.Portal>
);
