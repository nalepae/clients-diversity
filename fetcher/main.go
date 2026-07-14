package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/OffchainLabs/cl-dist/internal/aggregate"
	"github.com/OffchainLabs/cl-dist/internal/beacon"
	"github.com/OffchainLabs/cl-dist/internal/store"
)

const dailyRunHourUTC = 1

type config struct {
	beaconURL  string
	output     string
	startDate  string
	reqTimeout time.Duration
	maxRetries int
}

func loadConfig() config {
	config := config{
		beaconURL:  getenv("BEACON_URL", ""),
		output:     getenv("OUTPUT", "../web/data.json"),
		startDate:  getenv("START_DATE", ""),
		reqTimeout: time.Duration(getenvInt("REQ_TIMEOUT_SEC", 30)) * time.Second,
		maxRetries: getenvInt("MAX_RETRIES", 3),
	}

	flag.StringVar(&config.beaconURL, "beacon-url", config.beaconURL, "Beacon node REST base URL (e.g. http://localhost:3500)")
	flag.StringVar(&config.output, "output", config.output, "Path to the data.json store")
	flag.StringVar(&config.startDate, "start-date", config.startDate, "Backfill start date (YYYY-MM-DD, UTC) used on first run")
	flag.Parse()

	return config
}

func main() {
	cfg := loadConfig()
	if cfg.beaconURL == "" {
		log.Fatal("BEACON_URL (or -beacon-url) is required")
	}

	if cfg.startDate == "" {
		log.Fatal("START_DATE (or -start-date) is required")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	runDaemon(ctx, cfg)
}

func runDaemon(ctx context.Context, cfg config) {
	doRun := func() {
		if err := run(ctx, cfg); err != nil && ctx.Err() == nil {
			log.Printf("run failed: %v (will retry at next scheduled time)", err)
		}
	}

	doRun()
	for ctx.Err() == nil {
		next := nextRunUTC(time.Now())
		log.Printf("sleeping until %s (in %s)", next.Format(time.RFC3339), time.Until(next).Truncate(time.Second))
		timer := time.NewTimer(time.Until(next))

		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		doRun()
	}
}

func nextRunUTC(now time.Time) time.Time {
	n := now.UTC()
	run := n.Truncate(24 * time.Hour).Add(dailyRunHourUTC * time.Hour)
	if !run.After(n) {
		run = run.AddDate(0, 0, 1)
	}
	return run
}

func run(ctx context.Context, cfg config) error {
	df, err := store.Load(cfg.output)
	if err != nil {
		return fmt.Errorf("load: %w", err)
	}

	chain := aggregate.Mainnet()
	client := beacon.New(cfg.beaconURL, cfg.reqTimeout, cfg.maxRetries)

	finalizedSlot, err := client.FinalizedSlot(ctx)
	if err != nil {
		return fmt.Errorf("querying finalized checkpoint: %w", err)
	}

	// Determine the inclusive [from, to] UTC date range to process.
	from, err := resumeFrom(df, cfg.startDate)
	if err != nil {
		return fmt.Errorf("resumeFrom: %w", err)
	}

	to := time.Now().UTC().Truncate(24*time.Hour).AddDate(0, 0, -1) // yesterday; today is partial

	if from.After(to) {
		log.Printf(
			"up to date: nothing to process (last completed %s, target %s)",
			df.Meta.LastCompletedDate,
			aggregate.DateString(to),
		)

		writeMeta(df, cfg, df.Meta.LastCompletedDate)
		return store.Save(cfg.output, df)
	}

	log.Printf(
		"processing days %s..%s (finalized slot %d)",
		aggregate.DateString(from),
		aggregate.DateString(to),
		finalizedSlot,
	)

	for day := from; !day.After(to); day = day.AddDate(0, 0, 1) {
		if _, endSlot := chain.SlotRangeForDate(day); endSlot > finalizedSlot {
			log.Printf(
				"  %s not yet finalized (last slot %d > finalized %d); stopping",
				aggregate.DateString(day),
				endSlot,
				finalizedSlot,
			)

			break
		}

		rec, err := processDay(ctx, client, chain, day)
		if err != nil {
			writeMeta(df, cfg, df.Meta.LastCompletedDate)
			if err := store.Save(cfg.output, df); err != nil {
				log.Printf("save failed: %v", err)
			}

			return fmt.Errorf("processing %s: %w", aggregate.DateString(day), err)
		}

		df.AppendDay(rec)
		writeMeta(df, cfg, rec.Date)
		if err := store.Save(cfg.output, df); err != nil {
			return fmt.Errorf("save: %w", err)
		}

		log.Printf("  %s: total=%d identified=%d", rec.Date, rec.TotalBlocks, rec.IdentifiedBlocks)
	}

	log.Printf("done: %d day(s) now stored, last completed %s", len(df.Days), df.Meta.LastCompletedDate)
	return nil
}

func resumeFrom(df *store.DataFile, startDate string) (time.Time, error) {
	if df.Meta.LastCompletedDate != "" {
		last, err := aggregate.ParseDate(df.Meta.LastCompletedDate)
		if err != nil {
			return time.Time{}, fmt.Errorf("invalid lastCompletedDate %q: %w", df.Meta.LastCompletedDate, err)
		}
		return last.AddDate(0, 0, 1), nil
	}
	d, err := aggregate.ParseDate(startDate)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid start date %q: %w", startDate, err)
	}
	return d, nil
}

func processDay(ctx context.Context, client *beacon.Client, chain aggregate.Chain, date time.Time) (store.DayRecord, error) {
	startSlot, endSlot := chain.SlotRangeForDate(date)
	tally := aggregate.NewTally()

	for slot := startSlot; slot <= endSlot; slot++ {
		if err := ctx.Err(); err != nil {
			return store.DayRecord{}, err
		}

		g, found, err := client.GraffitiAtSlot(ctx, slot)
		if err != nil {
			return store.DayRecord{}, fmt.Errorf("graffiti at slot: %w", err)
		}

		tally.Add(aggregate.SlotResult{Found: found, Graffiti: g})
	}

	return tally.Record(date), nil
}

func writeMeta(df *store.DataFile, cfg config, lastCompleted string) {
	df.Meta.GeneratedAt = time.Now().UTC().Format(time.RFC3339)
	df.Meta.StartDate = cfg.startDate
	df.Meta.LastCompletedDate = lastCompleted
	df.Meta.GenesisTime = aggregate.MainnetGenesisTime
	df.Meta.SecondsPerSlot = aggregate.MainnetSecondsPerSlot
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}

	return def
}

func getenvInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}

	return def
}
