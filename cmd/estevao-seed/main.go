// Command estevao-seed builds the reference data of a new database from the
// canonical dataset in seeds/ (the Go counterpart of `rails db:seed`), and
// exports that dataset from a database seeded by Rails.
//
//	estevao-seed load   [-dir seeds]   fill an empty database (DATABASE_URL)
//	estevao-seed export [-dir seeds]   write the dataset from DATABASE_URL
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/dodopok/estevao-api-go/internal/seed"
)

func main() {
	if len(os.Args) < 2 || (os.Args[1] != "load" && os.Args[1] != "export") {
		fmt.Fprintln(os.Stderr, "usage: estevao-seed load|export [-dir seeds]")
		os.Exit(2)
	}
	fs := flag.NewFlagSet(os.Args[1], flag.ExitOnError)
	dir := fs.String("dir", "seeds", "dataset directory")
	_ = fs.Parse(os.Args[2:])
	ctx := context.Background()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		fmt.Fprintln(os.Stderr, "DATABASE_URL is required")
		os.Exit(2)
	}
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer pool.Close()
	switch os.Args[1] {
	case "export":
		err = seed.Export(ctx, pool, *dir)
	case "load":
		err = seed.Load(ctx, pool, *dir, func(format string, args ...any) { fmt.Printf(format+"\n", args...) })
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
