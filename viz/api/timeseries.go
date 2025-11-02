package api

import (
	"sort"
	"strings"
	"time"

	"github.com/example/cf-edgescout/store"
)

type TimeseriesPoint struct {
	Timestamp time.Time          `json:"timestamp"`
	Count     int                `json:"count"`
	Average   float64            `json:"average"`
	Regions   map[string]float64 `json:"regions,omitempty"`
}

func buildTimeseries(records []store.Record, bucket time.Duration) []TimeseriesPoint {
	if len(records) == 0 {
		return nil
	}

	sort.Slice(records, func(i, j int) bool {
		return records[i].Timestamp.Before(records[j].Timestamp)
	})

	start := records[0].Timestamp.Truncate(bucket)
	buckets := map[time.Time][]store.Record{}
	for _, record := range records {
		key := start.Add(record.Timestamp.Sub(start).Truncate(bucket))
		buckets[key] = append(buckets[key], record)
	}

	keys := make([]time.Time, 0, len(buckets))
	for key := range buckets {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i].Before(keys[j]) })

	points := make([]TimeseriesPoint, 0, len(keys))
	for _, key := range keys {
		group := buckets[key]
		var sum float64
		regions := map[string]struct {
			sum   float64
			count int
		}{}

		for _, record := range group {
			sum += record.Score
			region := strings.ToLower(regionOf(record))
			entry := regions[region]
			entry.sum += record.Score
			entry.count++
			regions[region] = entry
		}

		averages := map[string]float64{}
		for region, entry := range regions {
			if entry.count == 0 {
				continue
			}
			averages[region] = entry.sum / float64(entry.count)
		}

		points = append(points, TimeseriesPoint{
			Timestamp: key,
			Count:     len(group),
			Average:   sum / float64(len(group)),
			Regions:   averages,
		})
	}

	return points
}
