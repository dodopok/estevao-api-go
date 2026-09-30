// Command estevao is the operations tool of the Estêvão API: schema,
// reference data and maintenance tasks, the Go counterpart of the Rails
// app's rake tasks. The long-running processes are estevao-api and
// estevao-worker.
//
//	estevao db prepare        create/load/migrate (+ seed a new database); run on every deploy
//	estevao db migrate        apply pending migrations
//	estevao db status         list migrations and whether each is applied
//	estevao db rollback [-steps N]
//	estevao db new <name>     create db/migrations/<version>_<name>.sql
//	estevao db dump           rewrite db/schema.sql from DATABASE_URL (needs pg_dump)
//	estevao seed load|sync|export ...
//	estevao bible|flags|cache|books|notifications ...   operations (rake tasks)
package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	var err error
	switch os.Args[1] {
	case "db":
		err = dbCommand(os.Args[2:])
	case "seed":
		err = seedCommand(os.Args[2:])
	case "bible":
		err = bibleCommand(os.Args[2:])
	case "flags":
		err = flagsCommand(os.Args[2:])
	case "cache":
		err = cacheCommand(os.Args[2:])
	case "books":
		err = booksCommand(os.Args[2:])
	case "notifications":
		err = notificationsCommand(os.Args[2:])
	case "-h", "--help", "help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "estevao: unknown command %q\n", os.Args[1])
		usage()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "estevao: "+err.Error())
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `usage: estevao <command> [arguments]

  db prepare [-seeds DIR] [-no-seed]   create/load/migrate; seeds a database it created
  db migrate                           apply pending migrations
  db status                            migrations and their state
  db rollback [-steps N]               revert the last N migrations of db/migrations
  db new NAME [-dir db/migrations]     create a migration file
  db dump [-o db/schema.sql]           rewrite the schema snapshot (needs pg_dump)
  seed load [-dir seeds]               fill the reference tables of an empty database
  seed sync [-dir seeds] [-book CODE] [-apply]
                                       reconcile a live database with seeds/ (report only without -apply)
  seed export [-dir seeds]             write seeds/ from DATABASE_URL
  bible export -translation CODE [-o FILE]
  bible import -file FILE [-replace]   load a translation file (replaces rake bible:*)
  bible stats
  flags list | enable|disable|reset FEATURE [global | user TARGET]
                                       TARGET: e-mail, uid:<firebase uid> or id:<user id>
  cache clear [-pattern GLOB]          clear the Go caches (go/*), keeping rate limits and usage counters
  cache warm                           enqueue CacheWarmerJob
  books touch CODE...                  retire the caches of prayer books
  notifications test EMAIL             send a test push to a user

DATABASE_URL selects the database.`)
	os.Exit(2)
}

func databaseURL() (string, error) {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		return "", fmt.Errorf("DATABASE_URL is required")
	}
	return url, nil
}

func logf(format string, args ...any) { fmt.Printf(format+"\n", args...) }
