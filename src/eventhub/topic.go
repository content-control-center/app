package eventhub

import "strings"

// MatchTopic reports whether a subscriber's topic filter matches an
// event's topic.
//
// Three forms of filter:
//
//   - "all"       — catches every event (subject still to authz filter).
//   - "kind:id"   — exact match.
//   - "kind:*"    — prefix match; the "*" must be the last segment after
//     a colon. Matches "kind:anything" and "kind:nested:x".
//
// Empty event topics never match (treated as malformed).
func MatchTopic(filter, eventTopic string) bool {
	if eventTopic == "" {
		return false
	}
	if filter == "all" {
		return true
	}
	if filter == eventTopic {
		return true
	}
	if strings.HasSuffix(filter, ":*") {
		prefix := strings.TrimSuffix(filter, "*")
		return strings.HasPrefix(eventTopic, prefix)
	}
	return false
}

// MatchAny reports whether any of the filters matches eventTopic.
// Returns false on an empty filter list.
func MatchAny(filters []string, eventTopic string) bool {
	for _, f := range filters {
		if MatchTopic(f, eventTopic) {
			return true
		}
	}
	return false
}
