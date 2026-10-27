package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/internal/logmigration"
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
	printSchema := flag.Bool("print-schema", false, "print the current ClickHouse schema with TTL disabled")
	archivePath := flag.String("archive", "", "immutable JSONEachRow archive, all 22 columns, ascending source ID")
	manifestPath := flag.String("manifest", "", "verified source manifest JSON")
	insertRows := flag.Int("insert-rows", 1000, "maximum rows per INSERT (1..100000)")
	insertBytes := flag.Int("insert-bytes", 4*1024*1024, "maximum payload bytes per INSERT (1024..67108864)")
	pauseFile := flag.String("pause-file", "", "pause reading while this file exists")
	timeout := flag.Duration("timeout", 30*time.Minute, "maximum batch execution time")
	flag.Parse()
	if *printSchema {
		fmt.Println(model.ClickHouseLogCreateTableSQL(0))
		return nil
	}
	dsn := os.Getenv("LOG_IMPORT_CLICKHOUSE_DSN")
	if *archivePath == "" || *manifestPath == "" || dsn == "" || *timeout <= 0 || *timeout > time.Hour {
		return fmt.Errorf("provide -archive, -manifest, LOG_IMPORT_CLICKHOUSE_DSN and a timeout in (0, 1h]")
	}
	manifestBytes, err := os.ReadFile(*manifestPath)
	if err != nil {
		return err
	}
	var manifest model.ClickHouseLogImportManifest
	if err := common.Unmarshal(manifestBytes, &manifest); err != nil {
		return err
	}
	archive, err := os.Open(*archivePath)
	if err != nil {
		return err
	}
	defer archive.Close()
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
	ctx, cancel := context.WithTimeout(ctx, *timeout)
	defer cancel()
	input := logmigration.Reader{Context: ctx, Path: *pauseFile, Input: archive}
	result, err := model.ImportClickHouseLogsWithOptions(ctx, db, manifest, input, model.ClickHouseLogImportOptions{InsertRows: *insertRows, InsertBytes: *insertBytes})
	if err != nil {
		return err
	}
	encoded, err := common.Marshal(result)
	if err != nil {
		return err
	}
	fmt.Println(string(encoded))
	return nil
}
