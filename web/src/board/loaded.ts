/** The part of Excalidraw's API that says whether it has loaded its first scene. */
export interface Loading {
  getAppState(): { isLoading: boolean };
  onChange(cb: (elements: readonly unknown[], state: { isLoading: boolean }) => void): () => void;
}

/**
 * Resolves once the editor has loaded its first scene. Excalidraw hands out its
 * API before that, and loading replaces the scene with its initial data and
 * clears the history: a board put on it earlier is wiped, so the room is bound
 * only after. Its first change event comes once it has loaded.
 */
export function whenLoaded(api: Loading): Promise<void> {
  if (!api.getAppState().isLoading) return Promise.resolve();
  return new Promise((resolve) => {
    const off = api.onChange((_, state) => {
      if (state.isLoading) return;
      off();
      resolve();
    });
  });
}
