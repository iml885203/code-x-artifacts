// @vitest-environment node
import { describe, expect, it } from "vitest";
import { relativeSpan, relativeTime } from "./time";

const now = Date.parse("2026-10-20T12:00:00Z");
const ago = (ms: number) => new Date(now - ms).toISOString();
const S = 1000;
const MIN = 60 * S;
const H = 60 * MIN;
const D = 24 * H;

describe("relative wording", () => {
  const cases: [string, number, string, string][] = [
    ["a moment ago", 0, "now", "現在"],
    ["under ten seconds", 9 * S, "now", "現在"],
    ["tens of seconds", 34 * S, "30 seconds ago", "30 秒前"],
    ["a minute", 61 * S, "1 minute ago", "1 分鐘前"],
    ["minutes", 59 * MIN, "59 minutes ago", "59 分鐘前"],
    ["an hour", H, "1 hour ago", "1 小時前"],
    ["hours", 23 * H + 59 * MIN, "23 hours ago", "23 小時前"],
    ["yesterday", D, "yesterday", "昨天"],
    ["days", 6 * D, "6 days ago", "6 天前"],
    ["last week", 7 * D, "last week", "上週"],
    ["weeks", 29 * D, "4 weeks ago", "4 週前"],
    ["last month", 30 * D, "last month", "上個月"],
    ["months", 200 * D, "6 months ago", "6 個月前"],
    ["not yet a year", 364 * D, "11 months ago", "11 個月前"],
    ["last year", 365 * D, "last year", "去年"],
    ["years", 3 * 365 * D + D, "3 years ago", "3 年前"],
  ];
  it.each(cases)("%s", (_, ms, en, zh) => {
    expect(relativeTime(ago(ms), now, "en")).toBe(en);
    expect(relativeTime(ago(ms), now, "zh-Hant")).toBe(zh);
  });

  it("never shows a future time or negative value, whatever the clock skew", () => {
    for (const ahead of [1, 5 * S, 3 * MIN, 5 * H, 40 * D]) {
      const iso = new Date(now + ahead).toISOString();
      expect(relativeSpan(Date.parse(iso), now)).toEqual({ value: 0, unit: "second" });
      expect(relativeTime(iso, now, "en")).toBe("now");
      expect(relativeTime(iso, now, "zh-Hant")).toBe("現在");
    }
  });

  it("gives back text that is not a time instead of inventing one", () => {
    expect(relativeTime("not a time", now, "en")).toBe("not a time");
  });
});
