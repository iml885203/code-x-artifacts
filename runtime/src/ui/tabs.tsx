import * as React from "react";
import * as T from "@radix-ui/react-tabs";
import { cn } from "../lib/utils";
export const Tabs = T.Root;
export const TabsList = ({ className, ...p }: React.ComponentProps<typeof T.List>) => <T.List className={cn("inline-flex h-9 items-center rounded-lg bg-secondary p-1 text-muted-foreground", className)} {...p} />;
export const TabsTrigger = ({ className, ...p }: React.ComponentProps<typeof T.Trigger>) => <T.Trigger className={cn("inline-flex items-center rounded-md px-3 py-1 text-sm font-medium transition-all data-[state=active]:bg-background data-[state=active]:text-foreground", className)} {...p} />;
export const TabsContent = ({ className, ...p }: React.ComponentProps<typeof T.Content>) => <T.Content className={cn("mt-3", className)} {...p} />;
