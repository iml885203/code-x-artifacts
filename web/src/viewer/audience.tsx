import type { Visibility } from "../api";
import { Building, Globe, Lock } from "../icons/audience";
import type { Key } from "../i18n";

export const audiences: readonly Visibility[] = ["private", "organisation", "public"];

export const audienceLabel = {
  private: "visibility.private",
  organisation: "visibility.organisation",
  public: "visibility.public",
} as const satisfies Record<Visibility, Key>;

/** The full sentence of what each audience means (the share dialog's rows). */
export const audienceHint = {
  private: "visibility.private.hint",
  organisation: "visibility.organisation.hint",
  public: "visibility.public.hint",
} as const satisfies Record<Visibility, Key>;

export const audienceIcon = { private: Lock, organisation: Building, public: Globe } as const satisfies Record<Visibility, () => React.JSX.Element>;

export function isAudience(v: string): v is Visibility {
  return (audiences as readonly string[]).includes(v);
}
