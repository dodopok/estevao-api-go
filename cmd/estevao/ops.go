package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/dodopok/estevao-api-go/internal/bible"
	"github.com/dodopok/estevao-api-go/internal/config"
	"github.com/dodopok/estevao-api-go/internal/db"
	"github.com/dodopok/estevao-api-go/internal/features"
	"github.com/dodopok/estevao-api-go/internal/notify"
	"github.com/dodopok/estevao-api-go/internal/rediscache"
	"github.com/dodopok/estevao-api-go/internal/solidqueue"
	"github.com/dodopok/estevao-api-go/internal/users"
)

// connect opens the application's database (and Redis, when configured)
// the way the servers do.
func connect(ctx context.Context) error {
	url, err := databaseURL()
	if err != nil {
		return err
	}
	if err := db.Open(ctx, url, 4); err != nil {
		return err
	}
	if r := config.Get("REDIS_URL"); r != "" {
		return rediscache.Open(r)
	}
	return nil
}

// bibleCommand replaces rake bible:* (docs/OPERATIONS.md).
func bibleCommand(args []string) error {
	if len(args) == 0 {
		usage()
	}
	ctx := context.Background()
	url, err := databaseURL()
	if err != nil {
		return err
	}
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return err
	}
	defer pool.Close()
	fs := flag.NewFlagSet("bible "+args[0], flag.ExitOnError)
	translation := fs.String("translation", "", "translation code (e.g. nvi)")
	out := fs.String("o", "", "export: output file (default <translation>.jsonl.gz)")
	file := fs.String("file", "", "import: translation file")
	replace := fs.Bool("replace", false, "import: replace the verses already present")
	_ = fs.Parse(args[1:])
	switch args[0] {
	case "export":
		if *translation == "" {
			return fmt.Errorf("usage: estevao bible export -translation CODE [-o FILE]")
		}
		if *out == "" {
			*out = *translation + ".jsonl.gz"
		}
		f, err := os.Create(*out)
		if err != nil {
			return err
		}
		n, err := bible.ExportTranslation(ctx, pool, *translation, f)
		if cerr := f.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			os.Remove(*out)
			return err
		}
		logf("wrote %s: %d verses", *out, n)
		return nil
	case "import":
		if *file == "" {
			return fmt.Errorf("usage: estevao bible import -file FILE [-replace]")
		}
		f, err := os.Open(*file)
		if err != nil {
			return err
		}
		h, verses, warnings, err := bible.ReadTranslation(f)
		f.Close()
		if err != nil {
			return fmt.Errorf("%s: %w", *file, err)
		}
		for _, w := range warnings {
			logf("warning: %s", w)
		}
		var known bool
		if err := pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM bible_versions WHERE code = $1)`, h.Translation).Scan(&known); err != nil {
			return err
		}
		if !known {
			logf("warning: no bible_versions row for %q: add it to seeds/bible_versions.json and run estevao seed sync, or clients will not list it", h.Translation)
		}
		replaced, err := bible.ImportTranslation(ctx, pool, h.Translation, verses, *replace)
		if err != nil {
			return err
		}
		logf("imported %s: %d verses (%d replaced)", h.Translation, len(verses), replaced)
		return nil
	case "stats":
		stats, err := bible.Stats(ctx, pool)
		if err != nil {
			return err
		}
		var total int64
		for _, s := range stats {
			logf("%-16s %7d verses  %3d books", s.Translation, s.Verses, s.Books)
			total += s.Verses
		}
		logf("%d translations, %d verses", len(stats), total)
		return nil
	}
	return fmt.Errorf("unknown bible command %q (export, import, stats)", args[0])
}

// flagsCommand replaces rake feature_flags:*.
//
//	estevao flags list
//	estevao flags enable|disable|reset FEATURE [global | user TARGET]
func flagsCommand(args []string) error {
	if len(args) == 0 {
		usage()
	}
	ctx := context.Background()
	if err := connect(ctx); err != nil {
		return err
	}
	if args[0] == "list" {
		rows, err := features.ListOverrides(ctx)
		if err != nil {
			return err
		}
		if len(rows) == 0 {
			logf("Nenhum override persistido.")
		}
		for _, r := range rows {
			target := "global"
			if r.UserID != nil {
				email := ""
				if r.Email != nil {
					email = *r.Email
				}
				target = fmt.Sprintf("%s (user_id=%d)", email, *r.UserID)
			}
			state := "off"
			if r.Enabled {
				state = "on"
			}
			logf("%-28s %-8s %-45s %s", r.Feature, r.TargetType, target, state)
		}
		return nil
	}
	if len(args) < 2 {
		return fmt.Errorf("usage: estevao flags %s FEATURE [global | user TARGET]", args[0])
	}
	feature, err := features.ValidateFeature(args[1])
	if err != nil {
		return err
	}
	targetType, err := features.ValidateTarget(argAt(args, 2))
	if err != nil {
		return err
	}
	var user *users.User
	if targetType == "user" {
		if user, err = features.FindTargetUser(ctx, argAt(args, 3)); err != nil {
			return err
		}
	}
	target := "global"
	if user != nil {
		target = fmt.Sprintf("%s (user_id=%d)", user.Email, user.ID)
	}
	switch args[0] {
	case "enable", "disable":
		enabled := args[0] == "enable"
		if err := features.SetOverride(ctx, feature, targetType, user, enabled); err != nil {
			return err
		}
		verb := "Desabilitada"
		if enabled {
			verb = "Habilitada"
		}
		logf("%s: %s → %s", verb, feature, target)
	case "reset":
		removed, err := features.ResetOverride(ctx, feature, targetType, user)
		if err != nil {
			return err
		}
		if user != nil {
			target = user.Email
		}
		if removed {
			logf("Override removido: %s → %s", feature, target)
		} else {
			logf("Nenhum override encontrado: %s → %s", feature, target)
		}
	default:
		return fmt.Errorf("unknown flags command %q (list, enable, disable, reset)", args[0])
	}
	return nil
}

// cacheCommand replaces rake cache:clear_all / cache:clear_daily_office /
// cache:warm. Only the Go stack's entries (go/*) are cleared: rate limit
// budgets and the API key usage counters (not yet flushed to the
// database) live in the same Redis and must survive.
func cacheCommand(args []string) error {
	if len(args) == 0 {
		usage()
	}
	ctx := context.Background()
	if err := connect(ctx); err != nil {
		return err
	}
	switch args[0] {
	case "clear":
		fs := flag.NewFlagSet("cache clear", flag.ExitOnError)
		pattern := fs.String("pattern", "*", "glob under go/ (e.g. daily_office/* for rake cache:clear_daily_office)")
		_ = fs.Parse(args[1:])
		if rediscache.Client == nil {
			return fmt.Errorf("REDIS_URL is not set")
		}
		rediscache.DeleteRails(ctx, "go/"+*pattern)
		logf("cleared %s:go/%s", rediscache.Namespace, *pattern)
		return nil
	case "warm":
		enq, err := solidqueue.Enqueue(ctx, solidqueue.Job{Class: "CacheWarmerJob", Queue: "maintenance", Arguments: []any{}})
		if err != nil {
			return err
		}
		logf("enqueued CacheWarmerJob (job %s): the worker service performs it", enq.ActiveJobID)
		return nil
	}
	return fmt.Errorf("unknown cache command %q (clear, warm)", args[0])
}

// booksCommand replaces rake prayer_books:touch: moving a book's updated_at
// retires every cache keyed on it.
func booksCommand(args []string) error {
	if len(args) < 2 || args[0] != "touch" {
		return fmt.Errorf("usage: estevao books touch CODE [CODE...]")
	}
	ctx := context.Background()
	if err := connect(ctx); err != nil {
		return err
	}
	rows, err := db.Q().Query(ctx, `UPDATE prayer_books SET updated_at = $2 WHERE code = ANY($1) RETURNING code`, args[1:], users.Now())
	if err != nil {
		return err
	}
	defer rows.Close()
	touched := map[string]bool{}
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			return err
		}
		touched[c] = true
		logf("touched %s", c)
	}
	for _, c := range args[1:] {
		if !touched[c] {
			logf("no prayer book %s", c)
		}
	}
	return rows.Err()
}

// notificationsCommand replaces rake notifications:test_notification.
func notificationsCommand(args []string) error {
	if len(args) != 2 || args[0] != "test" {
		return fmt.Errorf("usage: estevao notifications test EMAIL")
	}
	ctx := context.Background()
	if err := connect(ctx); err != nil {
		return err
	}
	u, err := users.ByEmail(ctx, args[1])
	if err != nil {
		return err
	}
	if u == nil {
		return fmt.Errorf("Usuário com email '%s' não encontrado", args[1])
	}
	n, err := notify.ActiveTokens(ctx, u.ID)
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("Usuário não possui tokens FCM ativos")
	}
	logf("Enviando notificação de teste para %s...", u.Email)
	r := notify.SendAnnouncement(ctx, u, "Teste de Notificação 🔔", "Esta é uma notificação de teste do sistema Ordo!", nil)
	if !r.Sent {
		return fmt.Errorf("Erro ao enviar notificação: %s", r.Error)
	}
	logf("Notificação enviada com sucesso!")
	return nil
}

func argAt(args []string, i int) string {
	if i < len(args) {
		return strings.TrimSpace(args[i])
	}
	return ""
}
