import { Tooltip } from "./Tooltip";
import { formatTime } from "./format";
import { format, useLang, useT, type Key } from "./i18n";
import { relativeTime, useNow } from "./time";

/**
 * A time as people say it ("2 hours ago"). The exact local date and time is
 * the tooltip and part of the accessible name, so it is there for a pointer and
 * for a screen reader. `sentence` is a catalogue entry with {time} where the
 * wording goes ("Edited {time}"): the position differs between languages.
 */
export function RelativeTime({ iso, sentence }: { iso: string; sentence?: Key }) {
  const t = useT();
  const lang = useLang();
  const when = relativeTime(iso, useNow(), lang);
  const text = sentence ? format(t(sentence), { time: when }) : when;
  const exact = formatTime(iso);
  return (
    <Tooltip content={exact}><time dateTime={iso} tabIndex={0} aria-label={`${text}, ${exact}`}>
      {text}
    </time></Tooltip>
  );
}
