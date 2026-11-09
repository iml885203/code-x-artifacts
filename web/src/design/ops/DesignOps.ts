// What the person's keys and buttons do to the elements of a design: duplicate,
// copy and paste, group and ungroup, stacking order, and inserting. Each is one
// transaction of the room, so one step of their Undo, and live for everyone.
import type { Target } from "../controller";
import type { DesignEdits } from "../edits";
import { startOf } from "../geometry";
import { copyOf, duplicate, group, insertImage, insertShape, paste, restack, ungroup, type At, type Copied, type Item, type Outcome, type Picture, type Ref, type Shape, type Stack } from "./nodes";

const itemOf = (t: Target): Item => ({
  file: t.file,
  pid: t.pid,
  box: t.report.computed.position === "absolute" ? startOf(t.report.computed, t.report.rect) : null,
});

export class DesignOps {
  private clipboard: Copied[] = [];
  private pastes = 0;

  constructor(private readonly edits: DesignEdits) {}

  private run(write: (map: DesignEdits["map"]) => Outcome): Outcome {
    let outcome: Outcome = { refused: "nothing" };
    this.edits.transact((map) => {
      outcome = write(map);
    });
    return outcome;
  }

  duplicate(targets: Target[]): Outcome {
    return this.run((map) => duplicate(map, targets.map(itemOf)));
  }

  /** Remembers the elements as they are now; what is pasted later is that, whatever happens to them in between. */
  copy(targets: Target[]): boolean {
    this.clipboard = copyOf(this.edits.map, targets.map(itemOf));
    this.pastes = 0;
    return this.clipboard.length > 0;
  }

  get canPaste(): boolean {
    return this.clipboard.length > 0;
  }

  /** The frame the copy came from, for a paste with nothing selected. */
  get copiedFrom(): string | null {
    return this.clipboard[0]?.file ?? null;
  }

  paste(file: string): Outcome {
    const times = ++this.pastes;
    return this.run((map) => paste(map, this.clipboard, file, times));
  }

  group(targets: Target[]): Outcome {
    return this.run((map) => group(map, targets.map(itemOf)));
  }

  ungroup(targets: Target[]): Outcome {
    return this.run((map) => ungroup(map, targets.map(itemOf)));
  }

  restack(targets: Target[], how: Stack): boolean {
    let wrote = false;
    this.edits.transact((map) => {
      wrote = restack(map, targets, how);
    });
    return wrote;
  }

  insertShape(file: string, shape: Shape, at: At, text: string): Ref | null {
    let made: Ref | null = null;
    this.edits.transact((map) => {
      made = insertShape(map, file, shape, at, text);
    });
    return made;
  }

  insertImage(file: string, picture: Picture, at: At): Ref | null {
    let made: Ref | null = null;
    this.edits.transact((map) => {
      made = insertImage(map, file, picture, at);
    });
    return made;
  }
}
