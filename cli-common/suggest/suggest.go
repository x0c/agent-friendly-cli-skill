// Package suggest provides the scoring foundation for similar-name
// suggestions in zero-hit scenarios. It is the algorithmic layer of the
// "zero hits must be self-explanatory" UX contract; error envelope
// construction (error code / hint / next_commands) stays in each caller's
// repository — this package only produces an ordered list of names.
//
// The first suggestion is a security surface: callers (e.g. mytool) build
// the next_commands retry command from the first suggestion in the list,
// so a wrong first suggestion points the agent at the wrong target. Two
// hard guarantees are therefore locked by tests: a folded exact match
// must rank first and stand alone (the 100-point tier outranks every
// fuzzy tier, so no tie is possible); fuzzy-tier ordering is deterministic
// across the whole chain (score -> token hit count -> name length ->
// name alphabetical order, with no map-iteration-order dependence — same
// input, same output).
package suggest

import (
	"sort"
	"strings"
)

// DefaultStripSuffixes is the default list of platform domain suffixes
// stripped from both sides' names before comparison. Service full names
// typically carry a platform suffix (here the neutral "-prod") while
// users usually type the short name or a dot alias; stripping is
// comparison-only — the returned suggestion list still contains the
// original full names. Adopters inject their real suffixes via Options.
var DefaultStripSuffixes = []string{"-prod"}

// Options holds scoring options: the zero value is usable (equivalent to
// StripSuffixes = DefaultStripSuffixes).
type Options struct {
	// StripSuffixes is the list of domain suffixes stripped before
	// comparison; nil means DefaultStripSuffixes; pass []string{} to
	// explicitly disable stripping.
	StripSuffixes []string
}

// stripSuffixes resolves the effective suffix list: nil selects the
// default list; an empty slice explicitly disables stripping.
func (o Options) stripSuffixes() []string {
	if o.StripSuffixes == nil {
		return DefaultStripSuffixes
	}
	return o.StripSuffixes
}

// Tokenize splits a name into tokens by common separators (- _ . / and
// whitespace), for token-overlap similarity and within-tier refinement
// (token hit count).
func Tokenize(name string) []string {
	return strings.FieldsFunc(name, func(r rune) bool {
		return r == '-' || r == '_' || r == '.' || r == '/' || r == ' '
	})
}

// Similarity returns the similarity of a single pair of names:
// (score, token hit count). Score skeleton (unified scoring tiers):
//
//	100 identical after folding (including suffix stripping and separator
//	    folding, e.g. user.api vs user-api-prod)
//	  3 full prefix in either direction
//	  2 substring in either direction
//	  2 full containment after folding (concatenated form, e.g.
//	    syncmember vs sync-member-task)
//	  2 edit distance <= 2 (typo)
//	  2 common prefix >= 3 while neither is a full prefix of the other
//	    (typo net, covering near misses that share a long prefix, such as
//	    clyops-servcie vs clyops-service-paascrm)
//	  1 token overlap (any equal token)
//
// The second return value is the number of tokens shared with the input
// after tokenization, used for within-tier refinement below the skeleton
// (more token hits > fewer token hits) without changing skeleton
// priority. Returns (0, 0) when either name is blank, or becomes blank
// after domain-suffix stripping — the empty string is a prefix of every
// string, and without this guard meaningless pure-suffix names would be
// pushed into high score tiers.
func Similarity(input, name string, opts Options) (score, tokenHits int) {
	score, tokenHits, _ = similarity(input, name, opts)
	return score, tokenHits
}

// tierRank is the evidence-strength order within the same score tier: the
// skeleton checks short-circuit in this order, so an earlier hit is
// stronger evidence (substring > folded contains > edit distance > common
// prefix). It serves only as a secondary sort tiebreak (after score and
// token hit count, before name length) and never changes the score tier.
// Lesson from practice (mytool's service-not-found test): without this
// tiebreak, on typo input mall-ordr a longer noise name sharing only the
// weak prefix mall- (mall-search) would outrank by length-descending the
// true name matched by edit distance (mall-order), letting the first
// suggestion enter next_commands and point the agent at the wrong target
// (ruled a security defect on 2026-11-05).
const (
	tierExact        = 0 // folded identical (100 points)
	tierPrefix       = 1 // full prefix (3 points)
	tierSubstring    = 2 // substring (2 points)
	tierFoldContains = 3 // full containment after folding (2 points)
	tierEditDistance = 4 // edit distance <= 2 (2 points)
	tierCommonPrefix = 5 // common prefix >= 3 (2 points)
	tierTokenOverlap = 6 // token overlap (1 point)
	tierNone         = 7 // no match
)

// similarity is the full internal similarity: it additionally returns the
// evidence-strength rank within the same score tier, for RankNames' use as
// a secondary tiebreak. The skeleton's short-circuit order is exactly the
// evidence-strength order.
func similarity(input, name string, opts Options) (score, tokenHits, tier int) {
	if strings.TrimSpace(input) == "" || strings.TrimSpace(name) == "" {
		return 0, 0, tierNone
	}
	in, nm := strings.ToLower(input), strings.ToLower(name)
	// Suffix stripping is comparison-only: user-api-prod -> user-api.
	for _, suffix := range opts.stripSuffixes() {
		if suffix == "" {
			continue
		}
		in, nm = strings.TrimSuffix(in, suffix), strings.TrimSuffix(nm, suffix)
	}
	if in == "" || nm == "" {
		return 0, 0, tierNone
	}
	hits := sharedTokenCount(in, nm)
	inFold, nmFold := foldName(in), foldName(nm)
	if in == nm || inFold == nmFold {
		return 100, hits, tierExact
	}
	if strings.HasPrefix(nm, in) || strings.HasPrefix(in, nm) {
		return 3, hits, tierPrefix
	}
	if strings.Contains(nm, in) || strings.Contains(in, nm) {
		return 2, hits, tierSubstring
	}
	if strings.Contains(nmFold, inFold) || strings.Contains(inFold, nmFold) {
		return 2, hits, tierFoldContains
	}
	if EditDistance(in, nm) <= 2 {
		return 2, hits, tierEditDistance
	}
	if commonPrefixLen(in, nm) >= 3 {
		return 2, hits, tierCommonPrefix
	}
	if hits > 0 {
		return 1, hits, tierTokenOverlap
	}
	return 0, 0, tierNone
}

// RankNames picks from the candidate list the names similar to any of the
// inputs: each candidate takes its best (score, token hit count,
// evidence-strength rank) combination against any input, sorted by score
// descending -> token hit count descending -> evidence strength within the
// same score tier (substring > folded contains > edit distance > common
// prefix) -> name length descending -> name alphabetical ascending, then
// truncated to the first limit entries.
// The evidence-strength rank is a safety tiebreak: without it, at equal
// score and token hits, length-descending alone would let a longer noise
// name sharing only a weak prefix outrank the true name matched by edit
// distance, and the first suggestion (which goes straight into the
// caller's next_commands) would point at the wrong target.
// Returns nil when inputs or names is empty, when all inputs are blank, or
// when limit <= 0; blank candidate names are skipped. Original full names
// are returned (suffix stripping is comparison-only). The ordering chain
// involves no map iteration — same input, same output.
func RankNames(inputs, names []string, limit int, opts Options) []string {
	if limit <= 0 || len(inputs) == 0 || len(names) == 0 {
		return nil
	}
	validInputs := make([]string, 0, len(inputs))
	for _, input := range inputs {
		if strings.TrimSpace(input) != "" {
			validInputs = append(validInputs, input)
		}
	}
	if len(validInputs) == 0 {
		return nil
	}
	type scored struct {
		name      string
		score     int
		tokenHits int
		tier      int
	}
	list := make([]scored, 0, len(names))
	for _, name := range names {
		if strings.TrimSpace(name) == "" {
			continue
		}
		best := scored{name: name}
		for _, input := range validInputs {
			s, hits, tier := similarity(input, name, opts)
			if s > best.score || (s == best.score && (hits > best.tokenHits || (hits == best.tokenHits && tier < best.tier))) {
				best.score, best.tokenHits, best.tier = s, hits, tier
			}
		}
		if best.score > 0 {
			list = append(list, best)
		}
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].score != list[j].score {
			return list[i].score > list[j].score
		}
		if list[i].tokenHits != list[j].tokenHits {
			return list[i].tokenHits > list[j].tokenHits
		}
		if list[i].tier != list[j].tier {
			return list[i].tier < list[j].tier
		}
		if len(list[i].name) != len(list[j].name) {
			return len(list[i].name) > len(list[j].name)
		}
		return list[i].name < list[j].name
	})
	out := make([]string, 0, len(list))
	for _, item := range list {
		if len(out) >= limit {
			break
		}
		out = append(out, item.name)
	}
	return out
}

// CommonPrefix returns the longest common prefix of two names
// (case-insensitive), for truncated-keyword variant suggestions (e.g.
// mytool's service_not_found prefix + wildcard follow-up query command).
func CommonPrefix(a, b string) string {
	a, b = strings.ToLower(a), strings.ToLower(b)
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	i := 0
	for i < n && a[i] == b[i] {
		i++
	}
	return a[:i]
}

// TruncateVariants generates truncated-keyword variants (dropping trailing
// segments one hyphen at a time):
// rule-api-service -> [rule-api, rule], at most max variants; a
// single-segment keyword has no variants. Used for zero-hit follow-up
// queries against data sources without a complete inventory (e.g. mytool's
// CMDB); it only generates variant strings and performs no query.
func TruncateVariants(keyword string, max int) []string {
	segments := strings.Split(keyword, "-")
	if len(segments) < 2 || max <= 0 {
		return nil
	}
	variants := make([]string, 0, max)
	for count := len(segments) - 1; count >= 1 && len(variants) < max; count-- {
		variants = append(variants, strings.Join(segments[:count], "-"))
	}
	return variants
}

// foldName folds away the separator-shape differences of a name: it
// removes - _ . / and whitespace (the input should already be
// lowercased), for full-containment judgement across separators —
// syncmember, once folded, is still a full prefix of the folded form of
// sync-member-task; they are concatenated spellings of the same name.
func foldName(name string) string {
	var b strings.Builder
	for _, r := range name {
		switch r {
		case '-', '_', '.', '/', ' ':
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// sharedTokenCount counts the tokens shared by two names after
// tokenization (the weighting basis of token-overlap similarity: more
// token hits means more similar). The internal map is used for counting
// only and does not affect ordering determinism.
func sharedTokenCount(a, b string) int {
	at := map[string]bool{}
	for _, t := range Tokenize(a) {
		if t != "" {
			at[t] = true
		}
	}
	count := 0
	for _, t := range Tokenize(b) {
		if t != "" && at[t] {
			count++
		}
	}
	return count
}

// commonPrefixLen returns the length of the longest common prefix of two
// already-normalized names (in bytes, same basis as CommonPrefix).
func commonPrefixLen(a, b string) int {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	i := 0
	for i < n && a[i] == b[i] {
		i++
	}
	return i
}
