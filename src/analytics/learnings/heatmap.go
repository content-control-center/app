package learnings

import "sort"

// minHeatmapPosts is the floor below which the slot grid is too sparse to show.
const minHeatmapPosts = 5

// HeatCell is one day-of-week × hour slot.
type HeatCell struct {
	DayOfWeek int     `json:"day_of_week"` // 0=Sunday .. 6=Saturday (matches /best-times)
	Hour      int     `json:"hour"`
	Score     float64 `json:"score"` // normalised 0..1 (max slot = 1.0)
	PostCount int     `json:"post_count"`
	Median    float64 `json:"median"`
}

// Slot is the single strongest posting slot.
type Slot struct {
	DayOfWeek int `json:"day_of_week"`
	Hour      int `json:"hour"`
	PostCount int `json:"post_count"`
}

// Heatmap is the "When your posts land" section.
type Heatmap struct {
	InsufficientHistory bool       `json:"insufficient_history,omitempty"`
	Metric              string     `json:"metric,omitempty"`
	Cells               []HeatCell `json:"cells,omitempty"`
	Strongest           *Slot      `json:"strongest,omitempty"`
	MeasuredPosts       int        `json:"measured_posts,omitempty"`
}

type slotKey struct{ dow, hour int }

// buildHeatmap groups posts into (dow, hour) slots, scoring each by the
// normalised median of the chosen metric ("darker is better").
func buildHeatmap(posts []PostFact, metric string) *Heatmap {
	if len(posts) < minHeatmapPosts {
		return &Heatmap{InsufficientHistory: true}
	}
	buckets := map[slotKey][]int{}
	for _, p := range posts {
		k := slotKey{int(p.PublishedAt.UTC().Weekday()), p.PublishedAt.UTC().Hour()}
		buckets[k] = append(buckets[k], metricValue(p, metric))
	}

	cells := make([]HeatCell, 0, len(buckets))
	var maxMedian float64
	for k, vals := range buckets {
		m := medianInts(vals)
		if m > maxMedian {
			maxMedian = m
		}
		cells = append(cells, HeatCell{DayOfWeek: k.dow, Hour: k.hour, PostCount: len(vals), Median: round2(m)})
	}
	for i := range cells {
		if maxMedian > 0 {
			cells[i].Score = round4(cells[i].Median / maxMedian)
		}
	}
	// Stable order: day, then hour.
	sort.Slice(cells, func(i, j int) bool {
		if cells[i].DayOfWeek != cells[j].DayOfWeek {
			return cells[i].DayOfWeek < cells[j].DayOfWeek
		}
		return cells[i].Hour < cells[j].Hour
	})

	strongest := strongestSlot(cells)
	return &Heatmap{
		Metric:        metric,
		Cells:         cells,
		Strongest:     strongest,
		MeasuredPosts: len(posts),
	}
}

func strongestSlot(cells []HeatCell) *Slot {
	best := -1
	for i, c := range cells {
		if best == -1 || c.Score > cells[best].Score ||
			(c.Score == cells[best].Score && c.PostCount > cells[best].PostCount) {
			best = i
		}
	}
	if best == -1 {
		return nil
	}
	return &Slot{DayOfWeek: cells[best].DayOfWeek, Hour: cells[best].Hour, PostCount: cells[best].PostCount}
}

func medianInts(xs []int) float64 {
	if len(xs) == 0 {
		return 0
	}
	s := append([]int(nil), xs...)
	sort.Ints(s)
	n := len(s)
	if n%2 == 1 {
		return float64(s[n/2])
	}
	return float64(s[n/2-1]+s[n/2]) / 2
}
