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

// RangeSet extracts the IPv4 and IPv6 slices from the aggregated entries.
func (a AggregatedSet) RangeSet() RangeSet {
	var rs RangeSet
	for _, entry := range a.Entries {
		if entry.Network == nil {
			continue
		}
		if entry.Network.IP.To4() != nil {
			rs.IPv4 = append(rs.IPv4, cloneIPNet(entry.Network))
		} else {
			rs.IPv6 = append(rs.IPv6, cloneIPNet(entry.Network))
		}
	}
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
		sort.Slice(entry.Metadata, func(i, j int) bool {
			return entry.Metadata[i].Source < entry.Metadata[j].Source
		})
		entries = append(entries, RangeEntry{Network: cloneIPNet(entry.Network), Metadata: append([]RangeMetadata{}, entry.Metadata...)})
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
