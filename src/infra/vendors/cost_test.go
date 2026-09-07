package vendors

import "testing"

func TestCost_TokenBreakdown(t *testing.T) {
	// $3/1M input, $15/1M output, $0.30/1M cache-read.
	rates := Rates{
		KindInput:     3_000_000,
		KindOutput:    15_000_000,
		KindCacheRead: 300_000,
	}
	u := Usage{KindInput: 1200, KindOutput: 800, KindCacheRead: 400}
	// 1200*3e6/1e6 + 800*15e6/1e6 + 400*3e5/1e6
	//  = 3600 + 12000 + 120 = 15720 micros
	if got := Cost(u, rates); got != 15720 {
		t.Fatalf("Cost = %d, want 15720", got)
	}
}

func TestCost_MissingRateKindContributesZero(t *testing.T) {
	rates := Rates{KindInput: 3_000_000} // no output rate
	u := Usage{KindInput: 1_000_000, KindOutput: 5_000_000}
	// only input is priced: 1e6 * 3e6 / 1e6 = 3_000_000
	if got := Cost(u, rates); got != 3_000_000 {
		t.Fatalf("Cost = %d, want 3000000 (output unpriced)", got)
	}
}

func TestCost_ZeroAndEmpty(t *testing.T) {
	if got := Cost(nil, Rates{KindInput: 3_000_000}); got != 0 {
		t.Fatalf("Cost(nil) = %d, want 0", got)
	}
	if got := Cost(Usage{KindInput: 0}, Rates{KindInput: 3_000_000}); got != 0 {
		t.Fatalf("Cost(zero units) = %d, want 0", got)
	}
	if got := Cost(Usage{KindInput: 100}, nil); got != 0 {
		t.Fatalf("Cost(nil rates) = %d, want 0", got)
	}
}

func TestCost_PublisherPerAction(t *testing.T) {
	// $0.10 per post => 100_000 micros per post => 1e11 per 1M posts.
	rates := Rates{KindPost: 100_000_000_000}
	if got := Cost(Usage{KindPost: 1}, rates); got != 100_000 {
		t.Fatalf("Cost(1 post) = %d, want 100000 ($0.10)", got)
	}
	if got := Cost(Usage{KindPost: 3}, rates); got != 300_000 {
		t.Fatalf("Cost(3 posts) = %d, want 300000", got)
	}
}

func TestCost_TruncatesNotRounds(t *testing.T) {
	// 1 token at $1/1M = 1 micro exactly; 1 token at $0.50/1M = 0.5 -> truncates to 0.
	if got := Cost(Usage{KindInput: 1}, Rates{KindInput: 1_000_000}); got != 1 {
		t.Fatalf("Cost = %d, want 1", got)
	}
	if got := Cost(Usage{KindInput: 1}, Rates{KindInput: 500_000}); got != 0 {
		t.Fatalf("Cost = %d, want 0 (truncated)", got)
	}
}
