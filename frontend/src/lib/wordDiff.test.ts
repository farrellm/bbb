import { describe, expect, it } from "vitest";
import { type DiffSegment, wordCount, wordDiff } from "./wordDiff";

// wordDiff.ts is copied from nisaba, which has no tests. These cases are
// the same as the Go port's (backend/internal/bbb/worddiff_test.go), so the
// two implementations are held to the same answers.

const eq = (text: string): DiffSegment => ({ type: "equal", text });
const add = (text: string): DiffSegment => ({ type: "add", text });
const rm = (text: string): DiffSegment => ({ type: "remove", text });

// sides rebuilds before and after from segments (wordDiff.md's invariant).
function sides(segs: DiffSegment[]): [string, string] {
  let a = "";
  let b = "";
  for (const s of segs) {
    if (s.type !== "add") a += s.text;
    if (s.type !== "remove") b += s.text;
  }
  return [a, b];
}

function delta(before: string, after: string): [number, number] {
  let removed = 0;
  let added = 0;
  for (const s of wordDiff(before, after)) {
    if (s.type === "remove") removed += wordCount(s.text);
    if (s.type === "add") added += wordCount(s.text);
  }
  return [removed, added];
}

describe("wordDiff", () => {
  it.each<[string, string, string, DiffSegment[]]>([
    ["identical", "same text", "same text", [eq("same text")]],
    ["both empty", "", "", []],
    ["empty before", "", "new words", [add("new words")]],
    ["empty after", "old words", "", [rm("old words")]],
    [
      "adjacent words group",
      "the quick brown fox",
      "the slow red fox",
      [eq("the "), rm("quick brown"), add("slow red"), eq(" fox")],
    ],
    [
      "punctuation stays put",
      "room.",
      "room and smiled.",
      [eq("room"), add(" and smiled"), eq(".")],
    ],
    [
      "newline equality is not absorbed",
      "a\nb",
      "c\nd",
      [rm("a"), add("c"), eq("\n"), rm("b"), add("d")],
    ],
    [
      "preserved phrase stays",
      "one two three four five six",
      "uno two three four five seis",
      [
        rm("one"),
        add("uno"),
        eq(" two three four five "),
        rm("six"),
        add("seis"),
      ],
    ],
  ])("%s", (_name, a, b, want) => {
    const got = wordDiff(a, b);
    expect(got).toEqual(want);
    expect(sides(got)).toEqual([a, b]);
  });

  it.each([
    [
      "Volunteers needed for lunch on Mondays.",
      "Volunteers are needed for lunch on Mondays and Fridays!",
    ],
    ["Deadline: Friday, October 30", "Deadline: Monday, November 2"],
    ["A\n\nB C D\n\nE", "A\n\nB X D\n\nE F"],
    ["completely different text here", "nothing in common at all"],
  ])("rebuilds both sides of %j", (a, b) => {
    expect(sides(wordDiff(a, b))).toEqual([a, b]);
  });
});

describe("wordCount", () => {
  it.each<[string, number]>([
    ["", 0],
    ["don't stop", 2],
    [" , . ! ", 0],
    ["re-read the résumé", 3],
    ["8:45–9:45 a.m.", 6],
  ])("%j has %i words", (s, n) => {
    expect(wordCount(s)).toBe(n);
  });
});

describe("word deltas", () => {
  it("match the Go port", () => {
    expect(delta("Join us Friday.", "Join us Friday!")).toEqual([0, 0]);
    expect(delta("the quick brown fox", "the slow red fox")).toEqual([2, 2]);
    // " fox" is absorbed between two edits, so it counts on both sides.
    expect(delta("the quick brown fox", "the slow red fox jumps")).toEqual([
      3, 4,
    ]);
    expect(
      delta("Deadline: Sunday, November 1", "Deadline: Monday, November 2"),
    ).toEqual([3, 3]);
  });
});
