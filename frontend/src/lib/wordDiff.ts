// wordDiff computes a word-level diff between two strings, returning ordered
// segments suitable for an inline "track-changes" view. It's intentionally
// dependency-free (the repo keeps a lean dep list) and mirrors the scan-style
// utilities like responseSegments.ts. The algorithm is documented step by step
// in wordDiff.md — keep the two in sync.
//
// Tokens alternate between word runs and whitespace runs so the rendered output
// reconstructs the original spacing and newlines exactly. The diff itself is a
// classic longest-common-subsequence over the token arrays (run only on the
// middle left after trimming the common prefix/suffix), backtracked into
// equal/remove/add segments. When the word-level table would exceed the size
// cap, lines are diffed first as an alignment and each changed region is
// re-diffed at word level (diffLinesRefined). A final semantic-cleanup pass
// (groupChanges) merges fragmented edit runs into readable strike/insert
// blocks.

export type DiffSegment = { type: 'equal' | 'add' | 'remove'; text: string }

// Split into whitespace runs, word runs, and individual punctuation/symbol
// characters, e.g. "Hi, world!" -> ["Hi", ",", " ", "world", "!"]. A word run
// is alphanumeric plus internal apostrophes/hyphens, so "don't", "it’s" and
// "re-read" stay single tokens; leading/trailing marks are still separate
// punctuation tokens. The Unicode (u) flag keeps accented and non-Latin
// letters inside word tokens. Empty input yields no tokens.
function tokenize(s: string): string[] {
  return s.match(/\s+|[\p{L}\p{N}]+(?:['’-][\p{L}\p{N}]+)*|[^\s\p{L}\p{N}]/gu) ?? []
}

// Coarse fallback granularity for oversized inputs: whole lines and newline runs.
function tokenizeLines(s: string): string[] {
  return s.match(/\n+|[^\n]+/g) ?? []
}

// Cap on LCS table cells — 25M Int32 entries is ~100 MB, the most we're
// willing to allocate before dropping to a coarser diff granularity.
const maxLcsCells = 25_000_000

// trimCommonEnds strips the longest common token prefix and suffix of two
// token arrays, returning [prefixLength, middleA, middleB]. The suffix scan
// stops at the prefix boundary so the two never overlap.
function trimCommonEnds(a: string[], b: string[]): [number, string[], string[]] {
  let start = 0
  while (start < a.length && start < b.length && a[start] === b[start]) start++
  let endA = a.length
  let endB = b.length
  while (endA > start && endB > start && a[endA - 1] === b[endB - 1]) {
    endA--
    endB--
  }
  return [start, a.slice(start, endA), b.slice(start, endB)]
}

function fitsCap(a: string[], b: string[]): boolean {
  return (a.length + 1) * (b.length + 1) <= maxLcsCells
}

export function wordDiff(before: string, after: string): DiffSegment[] {
  const a = tokenize(before)
  const b = tokenize(after)

  // Trim the common token prefix and suffix so the O(n·m) LCS core only sees
  // the edited middle — the typical input here is a near-identical variant,
  // where this collapses the table to a sliver.
  const [start, midA, midB] = trimCommonEnds(a, b)

  const segments: DiffSegment[] = []
  const push = (type: DiffSegment['type'], text: string) => {
    if (!text) return
    const last = segments[segments.length - 1]
    if (last && last.type === type) last.text += text
    else segments.push({ type, text })
  }

  push('equal', a.slice(0, start).join(''))

  if (fitsCap(midA, midB)) {
    diffTokens(midA, midB, push)
  } else {
    // The word-level table would be too large — align at line granularity
    // instead, then refine each changed region back to word level.
    const lineA = tokenizeLines(midA.join(''))
    const lineB = tokenizeLines(midB.join(''))
    if (fitsCap(lineA, lineB)) {
      diffLinesRefined(lineA, lineB, push)
    } else {
      // Too large even line-by-line: give up on alignment, wholesale replace.
      push('remove', midA.join(''))
      push('add', midB.join(''))
    }
  }

  push('equal', a.slice(start + midA.length).join(''))

  return groupChanges(segments)
}

// diffLinesRefined runs the LCS core over line tokens, but uses the result as
// an alignment rather than the output: unchanged lines pass through as equal,
// and each maximal run of changed lines (a region bounded by matching lines
// or blank-line runs) is re-diffed at word granularity. Since edits are
// typically paragraph-local, each region is small enough for a full
// word-level table even when the document as a whole was not. Only a region
// that is itself over the cap degrades to a wholesale remove+add — a changed
// region has no common lines left, so there is nothing coarser to anchor on.
function diffLinesRefined(
  lineA: string[],
  lineB: string[],
  push: (type: DiffSegment['type'], text: string) => void,
) {
  let removed = ''
  let added = ''
  const flush = () => {
    if (!removed && !added) return
    const a = tokenize(removed)
    const b = tokenize(added)
    const [start, midA, midB] = trimCommonEnds(a, b)
    push('equal', a.slice(0, start).join(''))
    if (fitsCap(midA, midB)) {
      diffTokens(midA, midB, push)
    } else {
      push('remove', midA.join(''))
      push('add', midB.join(''))
    }
    push('equal', a.slice(start + midA.length).join(''))
    removed = ''
    added = ''
  }
  diffTokens(lineA, lineB, (type, text) => {
    if (type === 'equal') {
      flush()
      push('equal', text)
    } else if (type === 'remove') {
      removed += text
    } else {
      added += text
    }
  })
  flush()
}

// diffTokens runs the LCS + backtrack core over two token arrays, emitting
// segments through push. The table is a single flat Int32Array (row-major,
// width m+1) rather than number[][] — same algorithm, ~10x less memory.
function diffTokens(
  a: string[],
  b: string[],
  push: (type: DiffSegment['type'], text: string) => void,
) {
  const n = a.length
  const m = b.length
  const width = m + 1

  // LCS length table: lcs[i*width + j] = LCS of a[i:] and b[j:].
  const lcs = new Int32Array((n + 1) * width)
  for (let i = n - 1; i >= 0; i--) {
    for (let j = m - 1; j >= 0; j--) {
      lcs[i * width + j] =
        a[i] === b[j]
          ? lcs[(i + 1) * width + (j + 1)] + 1
          : Math.max(lcs[(i + 1) * width + j], lcs[i * width + (j + 1)])
    }
  }

  let i = 0
  let j = 0
  while (i < n && j < m) {
    if (a[i] === b[j]) {
      push('equal', a[i])
      i++
      j++
    } else if (lcs[(i + 1) * width + j] >= lcs[i * width + (j + 1)]) {
      push('remove', a[i])
      i++
    } else {
      push('add', b[j])
      j++
    }
  }
  while (i < n) push('remove', a[i++])
  while (j < m) push('add', b[j++])
}

// A chunk is groupChanges' working form: equal text, or a paired edit that
// renders as one remove block followed by one add block.
type Chunk = { type: 'equal'; text: string } | { type: 'edit'; remove: string; add: string }

// groupChanges merges runs of edits that the backtracking fragmented apart.
// Without it, changing several adjacent words reads as alternating add/remove
// (e.g. -quick +slow =" " -brown +red), and a heavy rewrite whose LCS matched
// stray common tokens ("the", a comma) reads as confetti. An equality
// sandwiched between two edits is absorbed into both sides when it wouldn't
// stand on its own:
//   - never when it contains a newline — absorbed text is rendered twice
//     (struck and inserted), which must not duplicate paragraph breaks, and a
//     line boundary is a natural end to an edit run anyway;
//   - always when it's whitespace-only (the space exists identically in the
//     before- and after-text);
//   - otherwise when it's small relative to the edits on BOTH sides
//     (word count <= each neighbor's larger side — the diff-match-patch
//     cleanupSemantic rule, adapted to words; pure punctuation counts 0 and
//     is always absorbed). Larger equalities stay put, so genuinely-unchanged
//     spans aren't swallowed.
// Absorption repeats until stable: each merge widens the neighboring edits,
// which can unlock absorbing the next equality over.
function groupChanges(segments: DiffSegment[]): DiffSegment[] {
  const chunks: Chunk[] = []
  for (const s of segments) {
    const last = chunks[chunks.length - 1]
    if (s.type === 'equal') chunks.push({ type: 'equal', text: s.text })
    else if (last && last.type === 'edit') last[s.type === 'remove' ? 'remove' : 'add'] += s.text
    else {
      chunks.push({
        type: 'edit',
        remove: s.type === 'remove' ? s.text : '',
        add: s.type === 'add' ? s.text : '',
      })
    }
  }

  let merged = true
  while (merged) {
    merged = false
    for (let i = 1; i < chunks.length - 1; i++) {
      const eq = chunks[i]
      const prev = chunks[i - 1]
      const next = chunks[i + 1]
      if (eq.type !== 'equal' || prev.type !== 'edit' || next.type !== 'edit') continue
      if (!absorbable(eq.text, prev, next)) continue
      prev.remove += eq.text + next.remove
      prev.add += eq.text + next.add
      chunks.splice(i, 2)
      merged = true
    }
  }

  const out: DiffSegment[] = []
  for (const c of chunks) {
    if (c.type === 'equal') out.push({ type: 'equal', text: c.text })
    else {
      if (c.remove) out.push({ type: 'remove', text: c.remove })
      if (c.add) out.push({ type: 'add', text: c.add })
    }
  }
  return out
}

function absorbable(
  eq: string,
  prev: { remove: string; add: string },
  next: { remove: string; add: string },
): boolean {
  if (eq.includes('\n')) return false
  if (/^\s+$/.test(eq)) return true
  const wc = wordCount(eq)
  return (
    wc <= Math.max(wordCount(prev.remove), wordCount(prev.add)) &&
    wc <= Math.max(wordCount(next.remove), wordCount(next.add))
  )
}

// wordCount counts word runs in a string (same word shape as tokenize, so
// "don't" is one word), ignoring whitespace and punctuation (a
// pure-punctuation segment contributes 0).
export function wordCount(s: string): number {
  return (s.match(/[\p{L}\p{N}]+(?:['’-][\p{L}\p{N}]+)*/gu) ?? []).length
}
