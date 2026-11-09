import { useT } from "../i18n";
import { initialOf, colorClassOf, keyOf, type Person } from "../presence/people";

/** Phone overflow keeps identity and preferences available without crowding the title. */
export function ViewerSecondary({ people }: { people: Person[] }) {
  const t = useT();
  return (
    <>
      {people.length > 0 && (
        <section className="viewer-people" aria-label={t("presence.label")}>
          <h2 className="menu-label">{t("presence.label")}</h2>
          <ul>
            {people.map((person) => (
              <li key={keyOf(person)}>
                <span className={`pr-face ${colorClassOf(person)}${person.guest === true ? " pr-guest" : ""}`} aria-hidden="true">{initialOf(person.name)}</span>
                <span>{person.name}{person.self ? ` (${t("presence.you")})` : ""}{person.agent ? ` · ${t("presence.agent")}` : ""}{person.guest === true ? ` · ${t("presence.guest")}` : ""}</span>
              </li>
            ))}
          </ul>
        </section>
      )}

    </>
  );
}
