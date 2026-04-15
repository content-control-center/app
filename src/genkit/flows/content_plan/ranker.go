package content_plan

import (
	"log"
	"math"
	"sort"
)

// rankByCosineSimilarity returns up to topN asset IDs ordered by descending
// cosine similarity to the query vector. Only IDs with a score >= minScore are
// included. A zero or mismatched vector scores as 0.
func rankByCosineSimilarity(query []float32, embeddings map[string][]float32, topN int, minScore float32) []string {
	type scored struct {
		id    string
		score float32
	}

	scores := make([]scored, 0, len(embeddings))
	for id, vec := range embeddings {
		s := cosineSimilarity(query, vec)
		log.Printf("content_plan: asset %s similarity=%.4f (threshold=%.4f, pass=%v)", id, s, minScore, s >= minScore)
		if s >= minScore {
			scores = append(scores, scored{id: id, score: s})
		}
	}

	sort.Slice(scores, func(i, j int) bool { return scores[i].score > scores[j].score })

	limit := topN
	if limit > len(scores) {
		limit = len(scores)
	}
	result := make([]string, limit)
	for i := range result {
		result[i] = scores[i].id
	}
	return result
}

func cosineSimilarity(a, b []float32) float32 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}
	denom := math.Sqrt(normA) * math.Sqrt(normB)
	if denom == 0 {
		return 0
	}
	return float32(dot / denom)
}
