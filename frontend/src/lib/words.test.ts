import { describe, expect, it } from "vitest";
import { headline, wordChange } from "./words";

describe("headline", () => {
  it("is a plain sentence", () => {
    expect(headline("new", "lower")).toBe("New in Lower School");
    expect(headline("edit", "all")).toBe("Edited in All School");
    expect(headline("removed", "upper")).toBe("Removed from Upper School");
    expect(headline("moved", "middle")).toBe("Moved to Middle School");
  });
});

describe("wordChange", () => {
  it("counts added and removed words", () => {
    expect(wordChange(12, 4)).toBe("12 words added, 4 removed");
    expect(wordChange(1, 0)).toBe("1 word added");
    expect(wordChange(0, 3)).toBe("3 words removed");
    expect(wordChange(0, 0)).toBe("punctuation or formatting changed");
  });
});
