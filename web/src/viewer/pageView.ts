/** Full-page presentation uses the authorized viewer route; raw pages remain untrusted. */
export function fullPageView(path: string): { version: number | null } | null {
  const query = new URLSearchParams(path.split("?")[1] ?? "");
  if (query.get("view") !== "full") return null;
  const requested = query.get("version");
  const version = requested !== null && /^[1-9]\d*$/.test(requested) ? Number(requested) : NaN;
  // An explicitly chosen version is pinned and read-only, even if it is currently the newest.
  return { version: Number.isSafeInteger(version) ? version : null };
}
