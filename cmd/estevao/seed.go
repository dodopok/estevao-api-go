package main

import (
	"context"
	"flag"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/dodopok/estevao-api-go/internal/seed"
)

func seedCommand(args []string) error {
	if len(args) == 0 {
		usage()
	}
	sub, args := args[0], args[1:]
	fs := flag.NewFlagSet("seed "+sub, flag.ExitOnError)
	dir := fs.String("dir", "seeds", "dataset directory")
	_ = fs.Parse(args)
	url, err := databaseURL()
	if err != nil {
		return err
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return err
	}
	defer pool.Close()
	switch sub {
	case "load":
		return seed.Load(ctx, pool, *dir, logf)
	case "export":
		return seed.Export(ctx, pool, *dir)
	}
	return fmt.Errorf("unknown seed command %q (load, export)", sub)
}
