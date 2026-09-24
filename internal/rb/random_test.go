package rb

import "testing"

// Reference values printed by Ruby 3.2 (Random.new(seed) then the same calls).
func TestRandomMatchesRuby(t *testing.T) {
	cases := map[uint64][6]int{
		0: {0, 6, 0, 835, 3, 3}, 1: {1, 4, 8, 767, 3, 1}, 42: {0, 4, 7, 700, 3, 4},
		4294967295: {1, 3, 7, 948, 3, 0}, 4294967296: {1, 7, 2, 85, 3, 4}, 8589934591: {1, 5, 1, 937, 3, 4},
		123456789012: {0, 1, 0, 907, 3, 1}, 1<<40 + 17: {0, 7, 3, 524, 3, 2},
		4294967297: {1, 3, 0, 964, 3, 1}, 12884901888: {1, 3, 5, 889, 3, 2}, 3000000000: {1, 6, 2, 302, 3, 3},
		7777777777: {0, 2, 2, 134, 3, 1}, 18446744073709551615: {0, 1, 0, 787, 3, 1},
	}
	for seed, want := range cases {
		r := NewRandom(seed)
		got := [6]int{r.RandRange(0, 1), r.RandRange(1, 7), r.RandRange(0, 9), r.RandRange(0, 999), r.RandRange(3, 3), r.RandRange(0, 4)}
		if got != want {
			t.Errorf("seed %d: got %v want %v", seed, got, want)
		}
	}
}
