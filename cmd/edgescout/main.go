package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/example/cf-edgescout/exporter"
	"github.com/example/cf-edgescout/fetcher"
	"github.com/example/cf-edgescout/prober"
	"github.com/example/cf-edgescout/sampler"
	"github.com/example/cf-edgescout/scheduler"
	"github.com/example/cf-edgescout/scorer"
	"github.com/example/cf-edgescout/store"
	api "github.com/example/cf-edgescout/viz/api"
)

func main() {
	log.SetFlags(0)
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	cmd := os.Args[1]
	switch cmd {
	case "scan":
		scanCmd(os.Args[2:])
	case "daemon":
		daemonCmd(os.Args[2:])
	case "serve":
		serveCmd(os.Args[2:])
	case "help", "-h", "--help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n", cmd)
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, "cf-edgescout commands:\n")
	fmt.Fprintf(os.Stderr, "  scan   Perform a one-off scan of Cloudflare edges\n")
	fmt.Fprintf(os.Stderr, "  daemon Continuously run scans at an interval\n")
	fmt.Fprintf(os.Stderr, "  serve  Serve stored results via HTTP\n")
}

func scanCmd(args []string) {
	fs := flag.NewFlagSet("scan", flag.ExitOnError)
	domain := fs.String("domain", "", "Target domain to probe")
	count := fs.Int("count", 32, "Number of candidates to probe")
	retries := fs.Int("retries", 1, "Probe retries on failure")
	rate := fs.Duration("rate", 200*time.Millisecond, "Delay between probes")
	sources := fs.String("sources", strings.Join(defaultSourceNames(), ","), "Comma-separated data sources to use")
	cacheDir := fs.String("cache-dir", "", "Directory to persist fetched range cache")
	parallel := fs.Int("parallel", 4, "Number of candidates to probe concurrently")
	jsonlPath := fs.String("jsonl", "", "Persist results to a JSONL file")
	csvPath := fs.String("csv", "", "Export results to a CSV file")
	providerList := fs.String("providers", "official,bestip,uouin", "Comma separated provider keys (use 'all' for every source)")
	fs.Parse(args)

	if *domain == "" {
		fs.Usage()
		log.Fatal("domain is required")
	}

	ctx := context.Background()
	f := fetcher.New(nil)
	if err := configureFetcher(f, *sources, *cacheDir); err != nil {
		log.Fatalf("configure fetcher: %v", err)
	}
	ranges, err := fetchRanges(ctx, f)
	providerKeys := parseProviderKeys(*providerList)
	providers, err := fetcher.FilterProviders(fetcher.DefaultProviders(), providerKeys)
	if err != nil {
		log.Fatalf("providers: %v", err)
	}
	f := fetcher.New(nil)
	sources, fetchErr := f.FetchAll(ctx, providers)
	if fetchErr != nil {
		log.Printf("数据源告警: %v", fetchErr)
	}
	if len(sources) == 0 {
		log.Fatal("未能获取任何可用数据源")
	}

	var st store.Store
	if *jsonlPath != "" {
		st = store.NewJSONL(*jsonlPath)
	} else {
		st = store.NewMemory()
	}

	sched := &scheduler.Scheduler{
		Sampler:     sampler.New(nil),
		Prober:      prober.New(*domain),
		Scorer:      scorer.New(),
		Store:       st,
		RateLimit:   *rate,
		Retries:     *retries,
		Parallelism: *parallel,
	}
	results, err := sched.Scan(ctx, sources, *domain, *count)
	if err != nil {
		log.Fatalf("scan: %v", err)
	}
	fmt.Printf("scanned %d candidates\n", len(results))

	if *csvPath != "" {
		records, err := st.List(ctx)
		if err != nil {
			log.Fatalf("list results: %v", err)
		}
		file, err := os.Create(*csvPath)
		if err != nil {
			log.Fatalf("create csv: %v", err)
		}
		defer file.Close()
		if err := exporter.ToCSV(records, file); err != nil {
			log.Fatalf("export csv: %v", err)
		}
		fmt.Printf("exported CSV to %s\n", *csvPath)
	}
}

func daemonCmd(args []string) {
	fs := flag.NewFlagSet("daemon", flag.ExitOnError)
	domain := fs.String("domain", "", "Target domain to probe")
	count := fs.Int("count", 32, "Number of candidates per scan")
	retries := fs.Int("retries", 1, "Probe retries on failure")
	rate := fs.Duration("rate", 200*time.Millisecond, "Delay between probes")
	interval := fs.Duration("interval", 5*time.Minute, "Interval between scans")
	sources := fs.String("sources", strings.Join(defaultSourceNames(), ","), "Comma-separated data sources to use")
	cacheDir := fs.String("cache-dir", "edges-cache", "Directory to persist fetched range cache")
	parallel := fs.Int("parallel", 4, "Number of candidates to probe concurrently")
	jsonlPath := fs.String("jsonl", "edges.jsonl", "Path to JSONL store")
	providerList := fs.String("providers", "official,bestip,uouin", "Comma separated provider keys (use 'all' for every source)")
	fs.Parse(args)

	if *domain == "" {
		fs.Usage()
		log.Fatal("domain is required")
	}

	ctx := context.Background()
	st := store.NewJSONL(*jsonlPath)
	sched := &scheduler.Scheduler{
		Sampler:     sampler.New(nil),
		Prober:      prober.New(*domain),
		Scorer:      scorer.New(),
		Store:       st,
		RateLimit:   *rate,
		Retries:     *retries,
		Parallelism: *parallel,
	}
	providerKeys := parseProviderKeys(*providerList)
	providers, err := fetcher.FilterProviders(fetcher.DefaultProviders(), providerKeys)
	if err != nil {
		log.Fatalf("providers: %v", err)
	}
	rangeFetcher := fetcher.New(nil)
	if err := configureFetcher(rangeFetcher, *sources, *cacheDir); err != nil {
		log.Fatalf("configure fetcher: %v", err)
	}
	fmt.Printf("starting daemon with interval %s\n", interval.String())
	fetchFunc := func(ctx context.Context) (fetcher.RangeSet, error) {
		return fetchRanges(ctx, rangeFetcher)
	}
	fetchFunc := func(ctx context.Context) ([]fetcher.SourceRange, error) {
		sources, fetchErr := rangeFetcher.FetchAll(ctx, providers)
		if fetchErr != nil {
			log.Printf("数据源告警: %v", fetchErr)
		}
		if len(sources) == 0 {
			return nil, fmt.Errorf("未能获取任何可用数据源")
		}
		return sources, nil
	}
	fmt.Printf("starting daemon with interval %s\n", interval.String())
	if err := sched.RunDaemon(ctx, fetchFunc, *domain, *count, *interval); err != nil {
		log.Fatalf("daemon stopped: %v", err)
	}
}

func serveCmd(args []string) {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	jsonlPath := fs.String("jsonl", "edges.jsonl", "JSONL store path")
	addr := fs.String("addr", ":8080", "Address to listen on")
	fs.Parse(args)

	st := store.NewJSONL(*jsonlPath)
	server := &api.Server{Store: st}
	fmt.Printf("serving results on %s\n", *addr)
	if err := http.ListenAndServe(*addr, server.Handler()); err != nil {
		log.Fatalf("serve: %v", err)
	}
}

func configureFetcher(f *fetcher.Fetcher, sourcesCSV, cacheDir string) error {
	if cacheDir != "" {
		f.SetCacheDir(cacheDir)
	}
	names := parseSourceList(sourcesCSV)
	if len(names) == 0 {
		names = defaultSourceNames()
	}
	return f.UseSourceNames(names)
}

func parseSourceList(value string) []string {
	segments := strings.Split(value, ",")
	out := make([]string, 0, len(segments))
	for _, segment := range segments {
		trimmed := strings.TrimSpace(segment)
		if trimmed == "" {
			continue
		}
		out = append(out, trimmed)
	}
	return out
}

func defaultSourceNames() []string {
	defaults := fetcher.DefaultSources()
	names := make([]string, 0, len(defaults))
	for _, cfg := range defaults {
		names = append(names, cfg.Name)
	}
	return names
}

func fetchRanges(ctx context.Context, f *fetcher.Fetcher) (fetcher.RangeSet, error) {
	aggregated, err := f.FetchAggregated(ctx)
	if err != nil {
		if len(aggregated.Entries) == 0 {
			return fetcher.RangeSet{}, err
		}
		log.Printf("warning: partial range fetch completed with errors: %v", err)
	}
	return aggregated.RangeSet(), nil
}
func parseProviderKeys(input string) []string {
	parts := strings.Split(input, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}
