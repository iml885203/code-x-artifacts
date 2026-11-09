import { readDropdownConfig, type DropdownConfig } from "../../schema";

export type { DropdownConfig } from "../../schema";
export const readConfig = readDropdownConfig;

/** Drafts use exactly the production parser's label and Unicode rules. */
export function validConfig(raw: unknown): raw is DropdownConfig {
  if (raw === null || typeof raw !== "object" || Array.isArray(raw)) return false;
  if (Object.keys(raw).length !== 2 || !("options" in raw) || !("value" in raw)) return false;
  try { return readDropdownConfig(JSON.stringify(raw)) !== null; } catch { return false; }
}

export const encodeConfig = (config: DropdownConfig): string => JSON.stringify({ options: config.options, value: config.value });
export const defaults = (): DropdownConfig => ({ options: ["Option 1", "Option 2"], value: "Option 1" });
