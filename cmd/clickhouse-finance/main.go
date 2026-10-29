package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/QuantumNous/new-api/model"
	"gorm.io/driver/clickhouse"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	startArg := flag.String("start", "", "first UTC day YYYY-MM-DD (inclusive)")
	endArg := flag.String("end", "", "UTC day YYYY-MM-DD (exclusive), at most 31 days per invocation")
	initialize := flag.Bool("init", false, "create the finance summary table; does not change logs")
	timeout := flag.Duration("day-timeout", 10*time.Minute, "maximum work per UTC day, up to 1h")
	flag.Parse()
	dsn := os.Getenv("LOG_FINANCE_CLICKHOUSE_DSN")
	if dsn == "" || *timeout <= 0 || *timeout > time.Hour {
		return fmt.Errorf("set LOG_FINANCE_CLICKHOUSE_DSN and day-timeout in (0,1h]")
	}
	var start, end time.Time
	if *startArg != "" || *endArg != "" {
		var err error
		start, err = time.Parse("2006-01-02", *startArg)
		if err != nil {
			return err
		}
		end, err = time.Parse("2006-01-02", *endArg)
		if err != nil {
			return err
		}
		if !end.After(start) || end.Sub(start) > 31*24*time.Hour || end.After(time.Now().UTC().Truncate(24*time.Hour)) {
			return fmt.Errorf("select 1..31 completed UTC days")
		}
	} else if !*initialize {
		return fmt.Errorf("provide -init or both -start and -end")
	}
	db, err := gorm.Open(clickhouse.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		return err
	}
	conn, err := db.DB()
	if err != nil {
		return err
	}
	defer conn.Close()
	conn.SetMaxOpenConns(2)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if *initialize {
		initCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		err = model.CreateClickHouseFinanceTable(db.WithContext(initCtx))
		cancel()
		if err != nil {
			return err
		}
	}
	for day := start; !start.IsZero() && day.Before(end); day = day.AddDate(0, 0, 1) {
		dayCtx, cancel := context.WithTimeout(ctx, *timeout)
		err = model.RebuildClickHouseFinanceDay(dayCtx, db, day.Unix())
		cancel()
		if err != nil {
			return fmt.Errorf("day %s incomplete: %w", day.Format("2006-01-02"), err)
		}
		fmt.Printf("published UTC day %s\n", day.Format("2006-01-02"))
	}
	return nil
}
