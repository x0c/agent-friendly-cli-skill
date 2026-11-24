package suggest

// EditDistance computes the edit distance (callers handle lowercase
// normalization beforehand; computed per rune with a two-row rolling
// array — service names are mostly ASCII, but per-rune computation guards
// against multibyte character misalignment). Ported from mytool's original
// queryEditDistance with behavior identical character for character.
func EditDistance(a, b string) int {
	ar, br := []rune(a), []rune(b)
	la, lb := len(ar), len(br)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}
	prev := make([]int, lb+1)
	curr := make([]int, lb+1)
	for j := 0; j <= lb; j++ {
		prev[j] = j
	}
	for i := 1; i <= la; i++ {
		curr[0] = i
		for j := 1; j <= lb; j++ {
			cost := 1
			if ar[i-1] == br[j-1] {
				cost = 0
			}
			m := curr[j-1] + 1
			if v := prev[j] + 1; v < m {
				m = v
			}
			if v := prev[j-1] + cost; v < m {
				m = v
			}
			curr[j] = m
		}
		prev, curr = curr, prev
	}
	return prev[lb]
}
