// What the Go server hands the page: which page to show, who is looking, and
// the interface text in the language it negotiated. It arrives as a JSON block
// in the HTML because the CSP forbids inline scripts.
import type { Artifact } from "./api";

export interface TokenItem {
  id: string;
  name: string;
  createdAt: string;
}

export interface Boot {
  page: string;
  lang: string;
  user: string;
  initials: string;
  /** Available profile name; the account identifier stays in user. */
  displayName?: string;
  /** Every language the server has, and the path the language switcher returns to. */
  langs: string[];
  here: string;
  strings: Record<string, string>;
  /** Whether this deployment lets an author choose "public". */
  publicLinks: boolean;
  /** The sandbox attribute of the iframe that shows an artifact: the server's own list, the one /raw sends as CSP. */
  frameSandbox: string;
  /** The export formats each kind declares, by kind; a kind that cannot export is absent. The Export menu lists these and nothing else. */
  exports: Record<string, { format: string; /** The browser makes this one (it prints the page) instead of the export route. */ browser?: boolean }[]>;
  /** Only the viewer page has this: the artifact, already checked against who is looking. */
  artifact?: Artifact;
  /** Only the sign-in page: where signing in leads, what went wrong (a catalogue key) and what was typed. */
  login?: { next: string; error: string; username: string; oidc?: { providerName: string; startUrl: string } };
  /** Only the connect page: the address its commands point at, and the person's tokens (no secrets). */
  connect?: { baseUrl: string; tokens: TokenItem[]; oauthEnabled?: boolean };
  /** Validated browser authorization request; fields include its consent-bound CSRF token. */
  oauth?: { clientName: string; clientId: string; scope: string; action: string; fields: Record<string, string> };
  /** Only the error page: its HTTP status. */
  status?: number;
}

export function readBoot(doc: Document): Boot {
  const block = doc.getElementById("pictor-boot");
  if (!block?.textContent) throw new Error("the page has no #pictor-boot block");
  const raw: unknown = JSON.parse(block.textContent);
  if (!isBoot(raw)) throw new Error("#pictor-boot is not the boot contract");
  return raw;
}

function isBoot(v: unknown): v is Boot {
  if (typeof v !== "object" || v === null) return false;
  const b = v as Record<string, unknown>;
  return (
    typeof b.page === "string" &&
    typeof b.lang === "string" &&
    typeof b.user === "string" &&
    typeof b.initials === "string" &&
    Array.isArray(b.langs) &&
    typeof b.here === "string" &&
    typeof b.strings === "object" &&
    b.strings !== null &&
    typeof b.publicLinks === "boolean" &&
    typeof b.frameSandbox === "string" &&
    typeof b.exports === "object" &&
    b.exports !== null
  );
}
