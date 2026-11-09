import { useEffect, useMemo, useRef } from "react";
import { ApiError } from "../../api";
import { format, useT } from "../../i18n";
import type { Target } from "../controller";
import type { DesignEdits } from "../edits";
import { DesignOps } from "./DesignOps";
import type { Outcome, Ref, Refusal, Shape, Stack } from "./nodes";
import { addPicture, type Seams } from "./picture";

interface Args {
  artifactId: string;
  edits: DesignEdits | null;
  /** Whether the person may change this version. */
  editable: boolean;
  /** The selected elements as the editor works on them. */
  targets: () => Target[];
  /** The frames on the page that are drawn. */
  frames: { id: string; file: string }[];
  /** Selects what an operation made. */
  select: (refs: Ref[]) => void;
  /** Says something in the canvas's status line. */
  say: (text: string) => void;
  /** Where the pictures go and how big they are; the browser's own unless a test says otherwise. */
  seams?: Seams;
}

export interface Operations {
  duplicate: () => void;
  copy: () => void;
  paste: () => void;
  group: () => void;
  ungroup: () => void;
  stack: (how: Stack) => void;
  /** A text box, rectangle or oval with its top left at a point of a frame (in the frame's pixels). */
  place: (shape: Shape, frameId: string, at: { x: number; y: number }) => void;
  /** Uploads a picture and puts it in the frame of the selection, or the first frame. */
  picture: (file: File) => Promise<void>;
}

/** Where the first picture goes in a frame, from its top left corner. */
const PICTURE_AT = 40;

/** The operations of the element keys and buttons, working on what is selected now; null for someone who may not edit. */
export function useDesignOps({ artifactId, edits, editable, targets, frames, select, say, seams }: Args): Operations | null {
  const t = useT();
  const ops = useMemo(() => (edits && editable ? new DesignOps(edits) : null), [edits, editable]);
  const held = useRef({ targets, frames, select, say, t, artifactId, seams });
  useEffect(() => {
    held.current = { targets, frames, select, say, t, artifactId, seams };
  });
  return useMemo(() => {
    if (!ops) return null;
    const refusal = (why: Refusal) => {
      const { t: words, say: tell } = held.current;
      tell(words(why === "apart" ? "ops.apart" : why === "text" ? "ops.hasText" : "ops.nothingToUngroup"));
    };
    const settle = (outcome: Outcome) => {
      if ("refused" in outcome) {
        refusal(outcome.refused);
        return;
      }
      held.current.say("");
      held.current.select(outcome.done);
    };
    const fileOfSelection = () => held.current.targets()[0]?.file ?? ops.copiedFrom ?? held.current.frames[0]?.file ?? null;
    return {
      duplicate: () => {
        const chosen = held.current.targets();
        if (chosen.length > 0) settle(ops.duplicate(chosen));
      },
      copy: () => {
        const chosen = held.current.targets();
        if (chosen.length > 0) ops.copy(chosen);
      },
      paste: () => {
        const file = fileOfSelection();
        if (file !== null && ops.canPaste) settle(ops.paste(file));
      },
      group: () => {
        const chosen = held.current.targets();
        if (chosen.length > 0) settle(ops.group(chosen));
      },
      ungroup: () => {
        const chosen = held.current.targets();
        if (chosen.length > 0) settle(ops.ungroup(chosen));
      },
      stack: (how) => {
        const chosen = held.current.targets();
        if (chosen.length > 0) ops.restack(chosen, how);
      },
      place: (shape, frameId, at) => {
        const file = held.current.frames.find((f) => f.id === frameId)?.file;
        if (!file) return;
        const made = ops.insertShape(file, shape, { left: Math.max(0, Math.round(at.x)), top: Math.max(0, Math.round(at.y)) }, "Text");
        held.current.say("");
        if (made) held.current.select([made]);
      },
      picture: async (file) => {
        const { frames: shown, t: words, say: tell, artifactId: id, seams: s } = held.current;
        const target = fileOfSelection() ?? shown[0]?.file;
        if (!target) {
          tell(words("ops.noFrame"));
          return;
        }
        tell(format(words("ops.uploading"), { name: file.name }));
        try {
          const picture = await addPicture(id, file, s);
          const made = ops.insertImage(target, picture, { left: PICTURE_AT, top: PICTURE_AT });
          tell("");
          if (made) held.current.select([made]);
        } catch (err) {
          if (err instanceof ApiError) tell(err.reason === "" ? format(words("ops.refusedNoReason"), { name: file.name }) : format(words("ops.refused"), { name: file.name, reason: err.reason }));
          else {
            console.error("add a picture", err);
            tell(format(words("ops.unreadable"), { name: file.name }));
          }
        }
      },
    };
  }, [ops]);
}
