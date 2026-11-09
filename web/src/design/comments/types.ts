export type CommentTarget = "available" | "unavailable" | "removed";

export interface DesignComments {
  threads: { id: string; anchor: Record<string, unknown> }[];
  onTargets?: (targets: Record<string, CommentTarget>) => void;
  active: string | null;
  onOpen: (id: string) => void;
  onCompose: (anchor: Record<string, unknown>) => void;
}
