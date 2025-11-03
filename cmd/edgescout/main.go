package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
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
	jsonlPath := fs.String("jsonl", "", "Persist results to a JSONL file")
	csvPath := fs.String("csv", "", "Export results to a CSV file")
	fs.Parse(args)

	if *domain == "" {
		fs.Usage()
		log.Fatal("domain is required")
	}

	ctx := context.Background()
	f := fetcher.New(nil)
	ranges, err := f.Fetch(ctx)
	if err != nil {
		log.Fatalf("fetch ranges: %v", err)
	}

	var st store.Store
	if *jsonlPath != "" {
		st = store.NewJSONL(*jsonlPath)
	} else {
		st = store.NewMemory()
	}

	sched := &scheduler.Scheduler{
		Sampler:   sampler.New(nil),
		Prober:    prober.New(*domain),
		Scorer:    scorer.New(),
		Store:     st,
		RateLimit: *rate,
		Retries:   *retries,
	}
	results, err := sched.Scan(ctx, ranges, *domain, *count)
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
	jsonlPath := fs.String("jsonl", "edges.jsonl", "Path to JSONL store")
	fs.Parse(args)

	if *domain == "" {
		fs.Usage()
		log.Fatal("domain is required")
	}

	ctx := context.Background()
	st := store.NewJSONL(*jsonlPath)
	sched := &scheduler.Scheduler{
		Sampler:   sampler.New(nil),
		Prober:    prober.New(*domain),
		Scorer:    scorer.New(),
		Store:     st,
		RateLimit: *rate,
		Retries:   *retries,
	}
	rangeFetcher := fetcher.New(nil)
	fmt.Printf("starting daemon with interval %s\n", interval.String())
	if err := sched.RunDaemon(ctx, rangeFetcher.Fetch, *domain, *count, *interval); err != nil {
		log.Fatalf("daemon stopped: %v", err)
	}
}

func serveCmd(args []string) {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	jsonlPath := fs.String("jsonl", "edges.jsonl", "JSONL store path")
	addr := fs.String("addr", ":8080", "Address to listen on")
	cacheTTL := fs.Duration("cache-ttl", 30*time.Second, "Cache refresh interval for API responses")
	defaultSources := fs.String("default-sources", "", "Comma-separated default source filters applied to API queries")
	defaultRegions := fs.String("default-regions", "", "Comma-separated default region filters applied to API queries")
	defaultScoreMin := fs.String("default-score-min", "", "Default minimum score filter")
	defaultScoreMax := fs.String("default-score-max", "", "Default maximum score filter")
	corsOrigins := fs.String("cors-origins", "*", "Comma-separated list of allowed CORS origins")
	fs.Parse(args)

	st := store.NewJSONL(*jsonlPath)
	filters := api.FilterOptions{}
	if src := strings.TrimSpace(*defaultSources); src != "" {
		filters.Sources = splitCSV(src)
	}
	if reg := strings.TrimSpace(*defaultRegions); reg != "" {
		filters.Regions = splitCSV(reg)
	}
	if v := strings.TrimSpace(*defaultScoreMin); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			filters.MinScore = &f
		} else {
			log.Fatalf("parse default-score-min: %v", err)
		}
	}
	if v := strings.TrimSpace(*defaultScoreMax); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			filters.MaxScore = &f
		} else {
			log.Fatalf("parse default-score-max: %v", err)
		}
	}
	server := &api.Server{
		Store:          st,
		CacheTTL:       *cacheTTL,
		DefaultFilters: filters,
		AllowedOrigins: rawSplit(*corsOrigins),
	}
	fmt.Printf("serving results on %s\n", *addr)
	if err := http.ListenAndServe(*addr, server.Handler()); err != nil {
		log.Fatalf("serve: %v", err)
	}
}

func splitCSV(input string) []string {
	parts := strings.Split(input, ",")
	var out []string
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			out = append(out, strings.ToLower(trimmed))
		}
	}
	return out
}

func rawSplit(input string) []string {
	parts := strings.Split(input, ",")
	var out []string
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	if len(out) == 0 {
		return []string{"*"}
	}
	return out
}
