package api

import (
	"math"
	"sort"
	"strings"
	"time"

	"github.com/example/cf-edgescout/store"
)

type Summary struct {
	Total      int              `json:"total"`
	UpdatedAt  time.Time        `json:"updated_at"`
	Score      ScoreSummary     `json:"score"`
	Sources    []GroupSummary   `json:"sources"`
	Regions    []GroupSummary   `json:"regions"`
	Components []ComponentScore `json:"components"`
	Recent     []store.Record   `json:"recent"`
}

type ScoreSummary struct {
	Average float64   `json:"average"`
	Min     float64   `json:"min"`
	Max     float64   `json:"max"`
	Median  float64   `json:"median"`
	Latest  time.Time `json:"latest"`
}

type GroupSummary struct {
	Key   string  `json:"key"`
	Count int     `json:"count"`
	Avg   float64 `json:"average"`
	Min   float64 `json:"min"`
	Max   float64 `json:"max"`
}

type ComponentScore struct {
	Key     string  `json:"key"`
	Average float64 `json:"average"`
}

type SourceDetail struct {
	Source     string           `json:"source"`
	Total      int              `json:"total"`
	Score      ScoreSummary     `json:"score"`
	Regions    []GroupSummary   `json:"regions"`
	Components []ComponentScore `json:"components"`
	Recent     []store.Record   `json:"recent"`
}

func buildSummary(records []store.Record) Summary {
	summary := Summary{
		Total: len(records),
	}
	if len(records) == 0 {
		return summary
	}

	summary.Score = summariseScores(records)
	summary.Sources = summariseGroups(records, sourceOf)
	summary.Regions = summariseGroups(records, regionOf)
	summary.Components = summariseComponents(records)
	summary.Recent = lastN(records, 10)
	summary.UpdatedAt = records[len(records)-1].Timestamp
	return summary
}

func buildSourceDetail(source string, records []store.Record) SourceDetail {
	detail := SourceDetail{Source: source, Total: len(records)}
	if len(records) == 0 {
		return detail
	}
	detail.Score = summariseScores(records)
	detail.Regions = summariseGroups(records, regionOf)
	detail.Components = summariseComponents(records)
	detail.Recent = lastN(records, 10)
	return detail
}

func summariseScores(records []store.Record) ScoreSummary {
	if len(records) == 0 {
		return ScoreSummary{}
	}

	scores := make([]float64, len(records))
	var sum float64
	min := math.MaxFloat64
	max := -math.MaxFloat64
	var latest time.Time

	for i, record := range records {
		scores[i] = record.Score
		sum += record.Score
		if record.Score < min {
			min = record.Score
		}
		if record.Score > max {
			max = record.Score
		}
		if record.Timestamp.After(latest) {
			latest = record.Timestamp
		}
	}

	sort.Float64s(scores)
	median := scores[len(scores)/2]
	if len(scores)%2 == 0 {
		median = (scores[len(scores)/2-1] + scores[len(scores)/2]) / 2
	}

	return ScoreSummary{
		Average: sum / float64(len(records)),
		Min:     min,
		Max:     max,
		Median:  median,
		Latest:  latest,
	}
}

func summariseGroups(records []store.Record, selector func(store.Record) string) []GroupSummary {
	if selector == nil {
		return nil
	}

	groups := map[string][]float64{}
	for _, record := range records {
		key := strings.ToLower(selector(record))
		if key == "" {
			key = "unknown"
		}
		groups[key] = append(groups[key], record.Score)
	}

	out := make([]GroupSummary, 0, len(groups))
	for key, scores := range groups {
		if len(scores) == 0 {
			continue
		}
		sort.Float64s(scores)
		avg := 0.0
		min := scores[0]
		max := scores[len(scores)-1]
		for _, score := range scores {
			avg += score
		}
		avg /= float64(len(scores))
		out = append(out, GroupSummary{Key: key, Count: len(scores), Avg: avg, Min: min, Max: max})
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Count == out[j].Count {
			return out[i].Key < out[j].Key
		}
		return out[i].Count > out[j].Count
	})
	return out
}

func summariseComponents(records []store.Record) []ComponentScore {
	if len(records) == 0 {
		return nil
	}

	totals := map[string]struct {
		sum   float64
		count int
	}{}

	for _, record := range records {
		for key, value := range record.Components {
			entry := totals[key]
			entry.sum += value
			entry.count++
			totals[key] = entry
		}
	}

	out := make([]ComponentScore, 0, len(totals))
	for key, entry := range totals {
		if entry.count == 0 {
			continue
		}
		out = append(out, ComponentScore{Key: key, Average: entry.sum / float64(entry.count)})
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].Key < out[j].Key
	})
	return out
}

func lastN(records []store.Record, n int) []store.Record {
	if n <= 0 || len(records) == 0 {
		return nil
	}
	if len(records) <= n {
		out := append([]store.Record(nil), records...)
		reverseRecords(out)
		return out
	}
	subset := records[len(records)-n:]
	out := make([]store.Record, len(subset))
	for i := range subset {
		out[i] = subset[len(subset)-1-i]
	}
	return out
}

func reverseRecords(records []store.Record) {
	for i, j := 0, len(records)-1; i < j; i, j = i+1, j-1 {
		records[i], records[j] = records[j], records[i]
	}
}
