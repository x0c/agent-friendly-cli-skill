package suggest

import (
	"reflect"
	"testing"
)

// similarityCase is a table-driven case for single-pair similarity: it
// locks the unified scoring tier skeleton and the tokenHits refinement.
type similarityCase struct {
	name       string
	input      string
	candidate  string
	wantScore  int
	wantTokens int
}

func TestSimilarityTiers(t *testing.T) {
	cases := []similarityCase{
		// Exact tier (100): case folding, separator folding, and suffix
		// stripping all reach exact agreement.
		{name: "mixed-case exact", input: "User-Api", candidate: "user-api", wantScore: 100, wantTokens: 2},
		{name: "dot alias folded exact", input: "user.api", candidate: "user-api-prod", wantScore: 100, wantTokens: 2},
		{name: "suffixed input folded exact", input: "user-api-prod", candidate: "user-api", wantScore: 100, wantTokens: 2},
		// Prefix tier (3): full prefix in either direction.
		{name: "input is name prefix", input: "user", candidate: "user-center", wantScore: 3, wantTokens: 1},
		{name: "name is input prefix", input: "syncmember-task", candidate: "syncmember", wantScore: 3, wantTokens: 1},
		// Substring tier (2): containment in either direction.
		{name: "input contained in name", input: "user-api", candidate: "mall-user-api-prod", wantScore: 2, wantTokens: 2},
		{name: "name contained in input", input: "user-api-service", candidate: "api-service", wantScore: 2, wantTokens: 2},
		// Folded-containment tier (2): concatenated forms.
		{name: "concatenated input folded containment", input: "syncmember", candidate: "sync-member-task", wantScore: 2, wantTokens: 0},
		{name: "underscore input folded containment", input: "User_MP", candidate: "user-mp-api-prod", wantScore: 2, wantTokens: 2},
		// Edit-distance tier (2): typos.
		{name: "edit distance 1", input: "servcie", candidate: "service", wantScore: 2, wantTokens: 0},
		{name: "edit distance 2", input: "clyops-servcie", candidate: "clyops-service", wantScore: 2, wantTokens: 1},
		// Common-prefix tier (2): neither is a full prefix of the other,
		// edit distance greater than 2, shared prefix of >= 3 characters.
		{name: "long common prefix typo net", input: "clyops-servcie", candidate: "clyops-service-paascrm", wantScore: 2, wantTokens: 1},
		{name: "dot alias vs sibling service", input: "user.api", candidate: "user-center-prod", wantScore: 2, wantTokens: 1},
		// Token-overlap tier (1).
		{name: "single-token overlap", input: "flow-service-host", candidate: "Custom-Tools-Service", wantScore: 1, wantTokens: 1},
		{name: "multi-token overlap", input: "flow-service-host", candidate: "mbp-flow-service-task", wantScore: 1, wantTokens: 2},
		// Zero score: no token / folding / prefix / edit-distance overlap
		// at all.
		{name: "completely unrelated", input: "zzz-qqq", candidate: "auth-api-mp", wantScore: 0, wantTokens: 0},
		{name: "edit distance above 2 with short prefix", input: "abcde", candidate: "abxyz", wantScore: 0, wantTokens: 0},
		{name: "scrambled name misses edit-distance tier", input: "syncmember", candidate: "member-sync", wantScore: 0, wantTokens: 0},
		// Blank and pure-suffix defenses.
		{name: "blank input", input: "   ", candidate: "user-api", wantScore: 0, wantTokens: 0},
		{name: "blank candidate", input: "user-api", candidate: "  ", wantScore: 0, wantTokens: 0},
		{name: "candidate is pure domain suffix", input: "user-api", candidate: "-prod", wantScore: 0, wantTokens: 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			score, hits := Similarity(tc.input, tc.candidate, Options{})
			if score != tc.wantScore || hits != tc.wantTokens {
				t.Fatalf("Similarity(%q, %q) = (%d, %d), want (%d, %d)",
					tc.input, tc.candidate, score, hits, tc.wantScore, tc.wantTokens)
			}
		})
	}
}

// TestSimilarityStripSuffixesOff locks the suffix-stripping switch
// semantics of Options: nil selects the default list; an empty slice
// explicitly disables stripping.
func TestSimilarityStripSuffixesOff(t *testing.T) {
	// Default (nil): user.api and user-api-prod agree after folding
	// = 100.
	if score, _ := Similarity("user.api", "user-api-prod", Options{}); score != 100 {
		t.Fatalf("default should strip the -prod suffix and score 100, got %d", score)
	}
	// Explicitly disabled: the suffix participates in comparison, so
	// user.api no longer agrees after folding.
	if score, _ := Similarity("user.api", "user-api-prod", Options{StripSuffixes: []string{}}); score == 100 {
		t.Fatalf("with stripping disabled it must not agree after folding and score 100, got %d", score)
	}
	// A custom list takes effect.
	if score, _ := Similarity("user-api", "user-api-paascrm", Options{StripSuffixes: []string{"-paascrm"}}); score != 100 {
		t.Fatalf("a custom suffix list should strip -paascrm and score 100, got %d", score)
	}
}

// rankCase is a table-driven case for ranking: it locks RankNames' tier
// priority, tokenHits refinement, length-descending fallback, and
// original-full-name return.
type rankCase struct {
	name   string
	inputs []string
	names  []string
	limit  int
	want   []string
}

func TestRankNames(t *testing.T) {
	cases := []rankCase{
		{
			name:   "folded exact ranks first and returns original full name",
			inputs: []string{"user.api"},
			names:  []string{"Custom-Tools-Service", "user-center-prod", "user-api-prod", "mall-user-api-prod"},
			limit:  5,
			// user-center and mall-user-api share the 2-point tier: a
			// folded-containment hit with 2 tokens outranks the
			// common-prefix tier's 1 token.
			want: []string{"user-api-prod", "mall-user-api-prod", "user-center-prod"},
		},
		{
			name:   "multi-token hits outrank single-token hits",
			inputs: []string{"flow-service-host"},
			names:  []string{"Custom-Tools-Service", "Data-Service-Center", "file-service", "user-service", "tsp-flow-slave-node.service", "mbp-flow-service-task"},
			limit:  5,
			// The two 2-token hits (1-point tier) come first;
			// single-token hits fall back to length descending, then
			// alphabetical ascending.
			want: []string{"tsp-flow-slave-node.service", "mbp-flow-service-task", "Custom-Tools-Service", "Data-Service-Center", "file-service"},
		},
		{
			name:   "typo net first place and unrelated names excluded",
			inputs: []string{"clyops-servcie"},
			names:  []string{"clyops-service-paascrm", "clyconfig-service-paascrm", "auth-api-mp", "jvs-risk-service"},
			limit:  8,
			// jvs-risk-service shares only the token service with the
			// correct spelling; the typo input servcie does not hit it.
			want: []string{"clyops-service-paascrm", "clyconfig-service-paascrm"},
		},
		{
			name:   "concatenated input prefix hit first with folded containment backup",
			inputs: []string{"syncmember"},
			names:  []string{"syncmember-task", "sync-member-task", "member-sync-prod", "task-center-prod", "async-task"},
			limit:  5,
			want:   []string{"syncmember-task", "sync-member-task"},
		},
		{
			name:   "underscore uppercase input refined to first place by tokenHits",
			inputs: []string{"User_MP"},
			names:  []string{"user-mp-api-prod", "user-center-prod", "mp-gateway-prod"},
			limit:  5,
			want:   []string{"user-mp-api-prod", "user-center-prod", "mp-gateway-prod"},
		},
		{
			name:   "equal score and hits falls back to length descending",
			inputs: []string{"user-api-prod"},
			names:  []string{"user-api", "user-api-prod", "user-center-prod"},
			limit:  5,
			want:   []string{"user-api-prod", "user-api", "user-center-prod"},
		},
		{
			name:   "typo first place not outranked by weak-prefix longer names",
			inputs: []string{"mall-ordr"},
			names:  []string{"mall-pay", "mall-search", "mall-order", "mall-user", "user-center", "order-center"},
			limit:  5,
			// mall-ordr is a typo of mall-order (edit distance 1);
			// mall-search/user/pay share only the weak prefix mall-
			// (same 2-point tier, same 1 token hit). By the
			// evidence-strength order the true name must rank first —
			// the first suggestion goes straight into the caller's
			// next_commands.
			want: []string{"mall-order", "mall-search", "mall-user", "mall-pay"},
		},
		{
			name:   "limit truncation",
			inputs: []string{"service"},
			names:  []string{"a-service", "b-service", "c-service"},
			limit:  2,
			// service is a suffix of all three candidates (substring
			// tier, 2 points); equal score, token hits, and length fall
			// back to alphabetical order.
			want: []string{"a-service", "b-service"},
		},
		{
			name:   "multiple inputs take the best combination from any",
			inputs: []string{"zzz", "user"},
			names:  []string{"user-api", "member-api"},
			limit:  5,
			want:   []string{"user-api"},
		},
		{
			name:   "zero hits returns empty",
			inputs: []string{"zzz-qqq"},
			names:  []string{"auth-api-mp", "jvs-risk-service"},
			limit:  8,
			want:   nil,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := RankNames(tc.inputs, tc.names, tc.limit, Options{})
			if len(got) != len(tc.want) {
				t.Fatalf("RankNames = %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("RankNames position %d = %q, want %q (full output %v)", i+1, got[i], tc.want[i], got)
				}
			}
		})
	}
}

// TestRankNamesBoundary locks boundary behavior: empty inputs, blank
// inputs, empty candidates, and invalid limits all return nil; blank
// candidate names are skipped.
func TestRankNamesBoundary(t *testing.T) {
	if got := RankNames(nil, []string{"user-api"}, 5, Options{}); got != nil {
		t.Fatalf("empty inputs should return nil, got %v", got)
	}
	if got := RankNames([]string{"  "}, []string{"user-api"}, 5, Options{}); got != nil {
		t.Fatalf("blank-only inputs should return nil, got %v", got)
	}
	if got := RankNames([]string{"user"}, nil, 5, Options{}); got != nil {
		t.Fatalf("empty candidates should return nil, got %v", got)
	}
	if got := RankNames([]string{"user"}, []string{"user-api"}, 0, Options{}); got != nil {
		t.Fatalf("limit=0 should return nil, got %v", got)
	}
	if got := RankNames([]string{"user"}, []string{"user-api"}, -1, Options{}); got != nil {
		t.Fatalf("limit<0 should return nil, got %v", got)
	}
	// Blank candidate names are skipped: excluded from the suggestion
	// list and from disturbing the ordering.
	if got := RankNames([]string{"user"}, []string{"  ", "", "user-api"}, 5, Options{}); len(got) != 1 || got[0] != "user-api" {
		t.Fatalf("blank candidate names should be skipped, got %v", got)
	}
	// limit=1 truncation takes effect.
	if got := RankNames([]string{"service"}, []string{"a-service", "b-service"}, 1, Options{}); len(got) != 1 || got[0] != "a-service" {
		t.Fatalf("limit=1 should keep only the first entry, got %v", got)
	}
}

// TestRankNamesExactFirstLocked locks hard guarantee one: when the
// candidate set contains both a folded-exact name and highly similar fuzzy
// names, RankNames[0] must be the exact name (the 100-point tier outranks
// every fuzzy tier with no tie possible; the first suggestion goes
// straight into the caller's next_commands).
func TestRankNamesExactFirstLocked(t *testing.T) {
	names := []string{
		"user-center-prod",   // common-prefix tier, 2 points
		"mall-user-api-prod", // folded containment, 2 points
		"user-apix-prod",     // edit distance 1, 2 points
		"user-api-prod",      // folded exact, 100 points
		"user-api",                 // folded exact, 100 points (bare name)
	}
	for i := 0; i < 100; i++ {
		got := RankNames([]string{"user.api"}, names, 5, Options{})
		if len(got) == 0 || got[0] != "user-api-prod" {
			t.Fatalf("run %d: the folded-exact name user-api-prod must rank first, got %v", i+1, got)
		}
	}
}

// TestRankNamesDeterministic locks hard guarantee two: running the
// same-score candidate set 100 times yields exactly equal output (length
// descending -> alphabetical ascending fallback, no map-iteration-order
// influence).
func TestRankNamesDeterministic(t *testing.T) {
	names := []string{"b-service", "a-service", "c-service", "d-service", "e-service", "f-service"}
	var first []string
	for i := 0; i < 100; i++ {
		got := RankNames([]string{"service"}, names, 6, Options{})
		if i == 0 {
			first = got
			continue
		}
		if !reflect.DeepEqual(first, got) {
			t.Fatalf("run %d differs from the first run: %v vs %v", i+1, got, first)
		}
	}
	// Equal score (substring tier), equal token hits, equal length ->
	// alphabetical ascending; deterministic across the whole chain.
	want := []string{"a-service", "b-service", "c-service", "d-service", "e-service", "f-service"}
	if !reflect.DeepEqual(first, want) {
		t.Fatalf("equal score and hits should order by length descending then alphabetical ascending, got %v, want %v", first, want)
	}
}

// TestCommonPrefix locks the pure common-prefix function
// (case-insensitive).
func TestCommonPrefix(t *testing.T) {
	cases := []struct {
		a, b, want string
	}{
		{"User-Api", "user-api-service", "user-api"},
		{"abc", "xyz", ""},
		{"", "abc", ""},
		{"abc", "", ""},
		{"syncmember", "sync-member-task", "sync"},
	}
	for _, tc := range cases {
		if got := CommonPrefix(tc.a, tc.b); got != tc.want {
			t.Fatalf("CommonPrefix(%q, %q) = %q, want %q", tc.a, tc.b, got, tc.want)
		}
	}
}

// TestTruncateVariants locks truncated-variant generation (trailing
// segments dropped one at a time, the cap, and no variants for a
// single-segment keyword).
func TestTruncateVariants(t *testing.T) {
	cases := []struct {
		keyword string
		max     int
		want    []string
	}{
		{"rule-api-service", 2, []string{"rule-api", "rule"}},
		{"rule-api-service", 1, []string{"rule-api"}},
		{"rule-api-service", 0, nil},
		{"rule", 2, nil},
		{"clyopen-paascrm", 2, []string{"clyopen"}},
	}
	for _, tc := range cases {
		if got := TruncateVariants(tc.keyword, tc.max); !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("TruncateVariants(%q, %d) = %v, want %v", tc.keyword, tc.max, got, tc.want)
		}
	}
}

// TestEditDistance locks the pure edit-distance function (including empty
// strings and multibyte-character boundaries).
func TestEditDistance(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"", "", 0},
		{"", "abc", 3},
		{"abc", "", 3},
		{"abc", "abc", 0},
		{"servcie", "service", 2},
		{"kitten", "sitting", 3},
		{"αβγδε", "αβγξζ", 2},
	}
	for _, tc := range cases {
		if got := EditDistance(tc.a, tc.b); got != tc.want {
			t.Fatalf("EditDistance(%q, %q) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}
