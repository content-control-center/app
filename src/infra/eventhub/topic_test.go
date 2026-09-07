package eventhub

import "testing"

func TestMatchTopic(t *testing.T) {
	tests := []struct {
		name   string
		filter string
		topic  string
		want   bool
	}{
		{"all matches anything", "all", "entity:post:abc", true},
		{"all matches simple topic", "all", "job:xyz", true},
		{"all does not match empty topic", "all", "", false},
		{"exact match", "entity:post:abc", "entity:post:abc", true},
		{"exact mismatch", "entity:post:abc", "entity:post:xyz", false},
		{"prefix wildcard matches direct child", "job:*", "job:abc", true},
		{"prefix wildcard matches nested", "job:*", "job:abc:detail", true},
		{"prefix wildcard does not match different namespace", "job:*", "entity:post:abc", false},
		{"prefix wildcard does not match the bare prefix", "job:*", "job", false},
		{"empty filter never matches", "", "anything", false},
		{"filter with no wildcard does not match prefix", "entity:post", "entity:post:abc", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MatchTopic(tt.filter, tt.topic)
			if got != tt.want {
				t.Errorf("MatchTopic(%q, %q) = %v; want %v", tt.filter, tt.topic, got, tt.want)
			}
		})
	}
}

func TestMatchAny(t *testing.T) {
	tests := []struct {
		name    string
		filters []string
		topic   string
		want    bool
	}{
		{"empty filter list matches nothing", nil, "anything", false},
		{"single matching filter", []string{"job:*"}, "job:abc", true},
		{"first filter matches", []string{"job:*", "entity:*"}, "job:abc", true},
		{"second filter matches", []string{"entity:*", "job:*"}, "job:abc", true},
		{"all wins over specific", []string{"all", "entity:post:foo"}, "anything:goes", true},
		{"none match", []string{"job:*", "entity:*"}, "user:alice", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MatchAny(tt.filters, tt.topic)
			if got != tt.want {
				t.Errorf("MatchAny(%v, %q) = %v; want %v", tt.filters, tt.topic, got, tt.want)
			}
		})
	}
}
