// Starting a blank doc, design or board, and renaming an artifact: the browser's two calls for #77.
import { call, type Artifact } from "../api";

export type Startable = "doc" | "design" | "board";

/** The person's own new private artifact, empty; `title` is what they will see until they rename it. */
export async function startNew(kind: Startable, title: string): Promise<Artifact> {
  return (await (await call("POST", "/api/new", { kind, title })).json()) as Artifact;
}

/** Gives an artifact the person may edit a new title; a refusal is an ApiError whose `reason` says why. */
export async function renameArtifact(id: string, title: string): Promise<Artifact> {
  return (await (await call("PUT", `/api/artifacts/${encodeURIComponent(id)}/title`, { title })).json()) as Artifact;
}
