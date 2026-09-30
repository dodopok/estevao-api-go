package audiogen

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/dodopok/estevao-api-go/internal/audio"
	"github.com/dodopok/estevao-api-go/internal/audioadmin"
	"github.com/dodopok/estevao-api-go/internal/clock"
	"github.com/dodopok/estevao-api-go/internal/db"
	"github.com/dodopok/estevao-api-go/internal/rb"
)

// --- Audio::ClipCleanup -----------------------------------------------------------------

// CleanupClip ports Audio::ClipCleanup#call: false when the clip is still
// used outside the usage filters and was kept.
func CleanupClip(ctx context.Context, clip *audioadmin.Clip, usageFilters *rb.Map) (bool, error) {
	if usageFilters.Len() > 0 {
		var conds []string
		args := []any{clip.ID}
		usageFilters.Each(func(k string, v any) {
			switch x := v.(type) {
			case nil:
				conds = append(conds, "audio_clip_usages."+k+" IS NULL")
			case []any:
				args = append(args, stringList(x))
				conds = append(conds, "audio_clip_usages."+k+" = ANY($"+strconv.Itoa(len(args))+")")
			default:
				args = append(args, rb.ToS(x))
				conds = append(conds, "audio_clip_usages."+k+" = $"+strconv.Itoa(len(args)))
			}
		})
		tag, err := db.Q().Exec(ctx, `DELETE FROM audio_clip_usages WHERE audio_clip_id = $1 AND `+strings.Join(conds, " AND "), args...)
		if err != nil {
			return false, err
		}
		if tag.RowsAffected() == 0 {
			return false, nil
		}
		var remaining bool
		if err := db.Q().QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM audio_clip_usages WHERE audio_clip_id = $1)`, clip.ID).Scan(&remaining); err != nil {
			return false, err
		}
		if remaining {
			return false, nil
		}
	}
	storage := audio.PlainStorage()
	if err := storage.Delete(ctx, clip.Filename); err != nil {
		return false, err
	}
	rows, err := db.Q().Query(ctx, `SELECT filename FROM audio_clip_candidates WHERE audio_clip_id = $1 ORDER BY id`, clip.ID)
	if err != nil {
		return false, err
	}
	var filenames []string
	for rows.Next() {
		var f *string
		if err := rows.Scan(&f); err != nil {
			rows.Close()
			return false, err
		}
		filenames = append(filenames, rb.ToS(rb.Deref(f)))
	}
	rows.Close()
	for _, f := range filenames {
		if err := storage.Delete(ctx, f); err != nil {
			return false, err
		}
	}
	err = db.Transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		for _, table := range []string{"audio_clip_usages", "user_audio_usages", "audio_clip_candidates"} {
			if _, err := tx.Exec(ctx, `DELETE FROM `+table+` WHERE audio_clip_id = $1`, clip.ID); err != nil {
				return err
			}
		}
		_, err := tx.Exec(ctx, `DELETE FROM audio_clips WHERE id = $1`, clip.ID)
		return err
	})
	return err == nil, err
}

func stringList(in []any) []string {
	out := make([]string, len(in))
	for i, v := range in {
		out[i] = rb.ToS(v)
	}
	return out
}

// --- Audio::ClipRegenerator ---------------------------------------------------------------

const maxCustomInstructions = 1000

// RegenerateCandidate ports ClipRegenerator#call_candidate: a reviewable
// replacement generated beside the canonical clip. additional is nil for
// AUTO_CUSTOMIZATION (the stored customization of the text, if any).
func RegenerateCandidate(ctx context.Context, clip *audioadmin.Clip, additional *string, operationID *int64) (id int64, characters int, err error) {
	lang := clip.Language
	provider := audio.Current(&lang)
	storage := audio.NewStorage(provider)
	normalized := audio.Normalize(clip.Text, clip.LineType, "", nil, provider.Language)
	if rb.BlankString(normalized) {
		return 0, 0, &rb.RubyError{Class: "ArgumentError", Message: "audio clip has no speakable text"}
	}
	key := audio.ClipKey(provider, normalized)
	var instructions string
	if additional == nil {
		var found *string
		err := db.Q().QueryRow(ctx, `SELECT instructions FROM audio_clip_customizations WHERE status = 'active' AND text_digest = $1
			AND provider = $2 AND model = $3 AND voice = $4 AND language = $5 LIMIT 1`,
			audio.TextDigest(normalized), provider.Name, provider.Model, provider.Voice, provider.Language).Scan(&found)
		if err != nil && !db.NoRows(err) {
			return 0, 0, err
		}
		instructions = rb.ToS(rb.Deref(found))
	} else {
		instructions = rb.Strip(*additional)
		if len([]rune(instructions)) > maxCustomInstructions {
			return 0, 0, &rb.RubyError{Class: "ArgumentError", Message: "custom audio instructions are too long"}
		}
	}
	var custom *audio.Customization
	if !rb.BlankString(instructions) {
		if !provider.SupportsCustomInstructions() {
			return 0, 0, &rb.RubyError{Class: "ArgumentError", Message: provider.Name + " does not support per-clip audio instructions"}
		}
		sum := sha256.Sum256([]byte(instructions))
		custom = &audio.Customization{Instructions: instructions, InstructionsSHA256: hex.EncodeToString(sum[:])}
	}
	filename := storage.CandidateFilenameFor(key)
	created := false
	defer func() {
		if !created {
			_ = storage.Delete(context.WithoutCancel(ctx), filename)
		}
	}()
	err = audio.WithGenerationLock(ctx, key, func() error {
		var note string
		if custom != nil {
			note = custom.Instructions
		}
		data, err := provider.Synthesize(ctx, normalized, note)
		if err != nil {
			return err
		}
		duration := audio.Mp3Duration(data)
		if err := storage.Store(ctx, filename, data); err != nil {
			return err
		}
		var customText, customDigest *string
		if custom != nil {
			customText, customDigest = &custom.Instructions, &custom.InstructionsSHA256
		}
		if msg := candidateInvalid(key, filename, normalized, provider); msg != "" {
			return &rb.RubyError{Class: "ActiveRecord::RecordInvalid", Message: "Validation failed: " + msg}
		}
		now := clock.Now().UTC().Truncate(time.Microsecond)
		characters = len([]rune(normalized))
		err = db.Q().QueryRow(ctx, `INSERT INTO audio_clip_candidates (audio_clip_id, audio_operation_id, key, filename, text, line_type,
			provider, voice, model, speed, duration, character_count, language, instructions_sha256, configuration_fingerprint,
			custom_instructions, custom_instructions_sha256, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $18) RETURNING id`,
			clip.ID, operationID, key, filename, normalized, clip.LineType, provider.Name, provider.Voice, provider.Model, provider.Speed,
			duration, characters, provider.Language, provider.InstructionsSHA256(), provider.ConfigurationFingerprint(&normalized),
			customText, customDigest, now).Scan(&id)
		if err != nil {
			return err
		}
		created = true
		return nil
	})
	return id, characters, err
}

// candidateInvalid ports AudioClipCandidate's presence validations.
func candidateInvalid(key, filename, text string, p *audio.Provider) string {
	var msgs []string
	for _, f := range []struct{ name, value string }{{"Key", key}, {"Filename", filename}, {"Text", text},
		{"Provider", p.Name}, {"Model", p.Model}, {"Voice", p.Voice}, {"Language", p.Language}} {
		if rb.BlankString(f.value) {
			msgs = append(msgs, f.name+" can't be blank")
		}
	}
	return strings.Join(msgs, ", ")
}
