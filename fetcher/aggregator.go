package fetcher

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

// RangeEntry combines a CIDR block with metadata contributed by sources.
type RangeEntry struct {
	Network  *net.IPNet      `json:"network"`
	Metadata []RangeMetadata `json:"metadata"`
}

// AggregatedSet represents the deduplicated result of all providers.
type AggregatedSet struct {
	Entries []RangeEntry `json:"entries"`
}

// RangeSet extracts the IPv4/IPv6 slices from the aggregated entries and groups
// them per upstream source so the sampler can apply policies later.
func (a AggregatedSet) RangeSet() RangeSet {
	rs := RangeSet{}
	perSource := map[string]*SourceRangeSet{}
	for _, entry := range a.Entries {
		if entry.Network == nil {
			continue
		}
		cloned := cloneIPNet(entry.Network)
		if cloned.IP.To4() != nil {
			rs.IPv4 = append(rs.IPv4, cloned)
		} else {
			rs.IPv6 = append(rs.IPv6, cloned)
		}
		for _, meta := range entry.Metadata {
			if meta.Source == "" {
				continue
			}
			sr, ok := perSource[meta.Source]
			if !ok {
				sr = &SourceRangeSet{Name: meta.Source, Credibility: meta.Credibility}
				perSource[meta.Source] = sr
			}
			if cloned.IP.To4() != nil {
				sr.IPv4 = append(sr.IPv4, cloneIPNet(cloned))
			} else {
				sr.IPv6 = append(sr.IPv6, cloneIPNet(cloned))
			}
		}
	}
	for _, sr := range perSource {
		rs.Sources = append(rs.Sources, *sr)
	}
	sort.Slice(rs.Sources, func(i, j int) bool {
		return rs.Sources[i].Name < rs.Sources[j].Name
	})
	return rs
}

// Aggregator deduplicates networks and enriches them with metadata.
type Aggregator struct {
	mu      sync.Mutex
	entries map[string]*RangeEntry
}

// NewAggregator builds an empty aggregator.
func NewAggregator() *Aggregator {
	return &Aggregator{entries: make(map[string]*RangeEntry)}
}

// Add merges the records into the aggregator.
func (a *Aggregator) Add(records []RangeRecord) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, record := range records {
		if record.Network == nil {
			continue
		}
		key := record.Network.String()
		entry, ok := a.entries[key]
		if !ok {
			entry = &RangeEntry{Network: cloneIPNet(record.Network)}
			a.entries[key] = entry
		}
		entry.Metadata = append(entry.Metadata, record.Metadata)
	}
}

// Result returns the aggregated set sorted by CIDR string for stability.
func (a *Aggregator) Result() AggregatedSet {
	a.mu.Lock()
	defer a.mu.Unlock()
	entries := make([]RangeEntry, 0, len(a.entries))
	for _, entry := range a.entries {
		meta := append([]RangeMetadata(nil), entry.Metadata...)
		sort.Slice(meta, func(i, j int) bool {
			if meta[i].Source == meta[j].Source {
				return meta[i].Endpoint < meta[j].Endpoint
			}
			return meta[i].Source < meta[j].Source
		})
		entries = append(entries, RangeEntry{Network: cloneIPNet(entry.Network), Metadata: meta})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Network.String() < entries[j].Network.String()
	})
	return AggregatedSet{Entries: entries}
}

// Persist writes the aggregated set to the provided cache directory.
func (a AggregatedSet) Persist(cacheDir string) error {
	if cacheDir == "" {
		return nil
	}
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return err
	}
	payload, err := json.MarshalIndent(a, "", "  ")
	if err != nil {
		return err
	}
	tmp := filepath.Join(cacheDir, "ranges.json.tmp")
	if err := os.WriteFile(tmp, payload, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(cacheDir, "ranges.json"))
}

// LoadAggregatedFromCache reads the cached aggregated set.
func LoadAggregatedFromCache(cacheDir string) (AggregatedSet, error) {
	if cacheDir == "" {
		return AggregatedSet{}, errors.New("cache directory not configured")
	}
	data, err := os.ReadFile(filepath.Join(cacheDir, "ranges.json"))
	if err != nil {
		return AggregatedSet{}, err
	}
	var set AggregatedSet
	if err := json.Unmarshal(data, &set); err != nil {
		return AggregatedSet{}, fmt.Errorf("decode cache: %w", err)
	}
	return set, nil
}
