# Word-level diff algorithm

`wordDiff.ts` computes a word-level diff between two strings and returns ordered
segments suitable for an inline "track-changes" view (used by the per-attribute
diff page, `BlockAttributeDiffPage.tsx`). It is intentionally dependency-free.

```ts
type DiffSegment = { type: 'equal' | 'add' | 'remove'; text: string }
function wordDiff(before: string, after: string): DiffSegment[]
```

Each segment marks a run of text that is unchanged (`equal`), present only in
`after` (`add`), or present only in `before` (`remove`). Concatenating the
`equal`+`remove` text reproduces `before`; concatenating `equal`+`add`
reproduces `after` — with one caveat: text _absorbed_ by the semantic-cleanup
pass (step 5) exists identically in both inputs and appears in both a `remove`
and an `add` segment, so the invariant is exact per side (drop the `add`
segments to get `before`, drop the `remove` segments to get `after`) even
though a naive `equal`-only concatenation would miss the absorbed spans.

## 1. Tokenize

Each input is split into a flat array of tokens, where every token is exactly
one of:

- a **whitespace run** — `\s+`
- a **word run** — `[\p{L}\p{N}]+(?:['’-][\p{L}\p{N}]+)*`: alphanumerics plus
  _internal_ apostrophes and hyphens, so `don't`, `it’s`, and `re-read` are
  single tokens (the Unicode `u` flag keeps accented and non-Latin letters
  inside one token, e.g. `résumé`)
- a **single punctuation/symbol character** — `[^\s\p{L}\p{N}]`

```ts
s.match(/\s+|[\p{L}\p{N}]+(?:['’-][\p{L}\p{N}]+)*|[^\s\p{L}\p{N}]/gu) ?? []
```

So `"Hi, world!"` → `["Hi", ",", " ", "world", "!"]`, and `"don't"` →
`["don't"]`.

Two consequences of this token granularity:

- **Whitespace is preserved as its own tokens**, so reconstructed output keeps
  the original spacing and newlines exactly (the view renders with
  `white-space: pre-wrap`).
- **Punctuation is separated from adjacent words**, so editing the text around a
  word doesn't mark the word itself as changed. `room.` → `room and smiled.`
  diffs as `room`(equal) + ` and smiled`(add) + `.`(equal), not a wholesale
  replacement. Only _internal_ `'`/`’`/`-` bind to a word: leading, trailing,
  or doubled marks (`'tis`, `well--known`) still split off as punctuation.

## 2. Trim the common prefix and suffix

Before any table is built, the longest common token prefix and suffix of the
two arrays are stripped and emitted directly as `equal` segments; the LCS core
runs only on the differing middle. The suffix scan stops at the prefix
boundary so the two never overlap.

This is the main defense against the LCS's `O(n·m)` cost: the typical input is
a near-identical variant of a long attribute value, where trimming collapses
the problem to the edited region.

## 3. Longest common subsequence (LCS)

Given the middle token arrays `a` (from `before`) and `b` (from `after`), build
a table where `lcs[i][j]` is the length of the longest common subsequence of
the suffixes `a[i:]` and `b[j:]`. Filled bottom-up from the ends:

```
lcs[i][j] = a[i] === b[j]
  ? lcs[i+1][j+1] + 1
  : max(lcs[i+1][j], lcs[i][j+1])
```

Tokens are compared by exact string equality. The LCS is the set of tokens that
stay put; everything else is an insertion or deletion. The table is stored as a
single flat `Int32Array` of `(n+1)·(m+1)` cells (row-major) rather than
`number[][]` — same algorithm, roughly an order of magnitude less memory.

### Size guard

The table is only allocated up to a cap (`maxLcsCells`, 25M cells ≈ 100 MB).
If the trimmed middle would exceed it, the middle is re-tokenized at **line
granularity** (`/\n+|[^\n]+/` — whole lines and newline runs) and the LCS runs
over lines instead of words — but as an **alignment**, not as the output
(`diffLinesRefined`). Unchanged lines pass through as `equal`; each maximal
run of changed lines (a region bounded by matching lines or blank-line runs,
so typically one edited paragraph) is then re-diffed at **word granularity**:
its removed and added text are word-tokenized, prefix/suffix-trimmed, and run
through the same LCS core. Edits are usually paragraph-local, so each region's
table is tiny even when the whole document blew the cap — the output stays
word-level instead of striking entire paragraphs.

Two bounded degradations remain: a single changed region that itself exceeds
the cap (a massive rewrite with no common lines to anchor on) is emitted as
one wholesale `remove` + `add` pair for that region, and in the pathological
case where even the line-level table would exceed the cap, the entire middle
is emitted wholesale. All paths preserve the reconstruction invariant.

## 4. Backtrack into segments

Walk forward from `(0, 0)` using the table to decide each step:

- `a[i] === b[j]` → emit `equal`, advance both `i` and `j`.
- else if `lcs[i+1][j] >= lcs[i][j+1]` → emit `remove` (token only in `before`),
  advance `i`.
- else → emit `add` (token only in `after`), advance `j`.

After one array is exhausted, the remaining `a` tokens are `remove`s and the
remaining `b` tokens are `add`s. The `>=` tie-break makes deletions precede
insertions at an equal-cost fork, which keeps the output deterministic.

### Coalescing

Adjacent tokens of the same type are merged into one segment as they're emitted
(the new token's text is appended to the previous segment when types match).
This turns token-level output into readable runs — e.g. a contiguous insertion
of several words/spaces becomes a single `add` segment rather than many.

## 5. Semantic cleanup (`groupChanges`)

Two artifacts make the raw backtrack output hard to read:

- Inter-word whitespace tokens match, so a change spanning adjacent words
  fragments into edits separated by tiny `equal` spaces —
  `the quick brown fox` → `the slow red fox` backtracks to
  `="the " −"quick" +"slow" =" " −"brown" +"red" =" fox"`.
- The LCS maximizes matched tokens, so a heavy rewrite still matches stray
  common tokens (`the`, `a`, a comma), splintering what a human reads as "this
  passage was replaced" into confetti of alternating strikes and inserts.

`groupChanges` fixes both. It pairs each maximal run of edits into **one
`remove` block followed by one `add` block**, and absorbs an `equal` that sits
between two edits into both sides when it wouldn't stand on its own:

- **never** when it contains a newline — absorbed text renders twice (once
  struck, once inserted), which must not duplicate paragraph breaks; a line
  boundary is a natural end to an edit run anyway;
- **always** when it's whitespace-only (the space exists identically in the
  before- and after-text);
- otherwise when it's **small relative to the edits on both sides**: its word
  count is ≤ each neighboring edit's larger side (`max(wordCount(remove),
wordCount(add))`). This is diff-match-patch's `cleanupSemantic` rule adapted
  to word counts; a pure-punctuation equality (a matched comma) counts 0 words
  and is always absorbed between edits.

Absorption repeats until stable, because each merge widens the neighboring
edits and can unlock absorbing the next equality over. The first example
becomes `="the " −"quick brown" +"slow red" =" fox"`; a rewrite that shares
only function words collapses to a single strike + insert pair.

Equalities that fail the rule — a preserved phrase between two edits — stay
`equal` boundaries, so genuinely-unchanged spans are never swallowed.

## 6. Word counts

`wordCount(s)` counts word runs only (the same word shape as the tokenizer, so
`don't` is one word), ignoring whitespace and punctuation. The diff page sums
it over the `add` and `remove` segments to show a `−N / +M words` summary, so
a punctuation-only change (e.g. a stray `.`) contributes `0` and isn't reported
as a changed word. Because absorbed equalities land in both the `remove` and
`add` segments, the counts describe the _rendered markup_ — exactly the words
shown struck and inserted — not a minimal edit script.

## Complexity & edge cases

- **Time / space:** `O(n·m)` in the trimmed middle, capped at `maxLcsCells`
  table cells (beyond the cap: line-level alignment with per-region word
  refinement, then wholesale, each region and table individually capped).
  Prefix/suffix trimming makes the common case — a long value with a localized
  edit — effectively linear.
- **Empty inputs:** `tokenize("")` yields `[]`. Empty `before` → everything is
  `add`; empty `after` → everything is `remove`; both empty → `[]`.
- **Identical inputs** collapse to a single `equal` segment (fully consumed by
  the prefix trim).
