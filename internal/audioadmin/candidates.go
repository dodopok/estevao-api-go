package audioadmin

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/dodopok/estevao-api-go/internal/audio"
	"github.com/dodopok/estevao-api-go/internal/clock"
	"github.com/dodopok/estevao-api-go/internal/db"
	"github.com/dodopok/estevao-api-go/internal/rb"
)

// Candidate is an audio_clip_candidates row.
type Candidate struct {
	ID, AudioClipID                                       int64
	Key, Filename, Text, LineType, Provider, Model, Voice string
	Language, Status                                      string
	Speed                                                 float64
	Duration                                              *float64
	CharacterCount                                        int64
	InstructionsSHA, Fingerprint, CustomInstructionsSHA   *string
	CustomInstructions                                    *string
}

const candidateColumns = `id, audio_clip_id, key, filename, text, line_type, provider, model, voice, language, status, speed,
 duration, character_count, instructions_sha256, configuration_fingerprint, custom_instructions_sha256, custom_instructions`

func scanCandidate(row pgx.Row) (*Candidate, error) {
	c := &Candidate{}
	err := row.Scan(&c.ID, &c.AudioClipID, &c.Key, &c.Filename, &c.Text, &c.LineType, &c.Provider, &c.Model, &c.Voice,
		&c.Language, &c.Status, &c.Speed, &c.Duration, &c.CharacterCount, &c.InstructionsSHA, &c.Fingerprint,
		&c.CustomInstructionsSHA, &c.CustomInstructions)
	if db.NoRows(err) {
		return nil, nil
	}
	return c, err
}

// FindCandidate ports clip.candidates.find(id) (nil when absent).
func FindCandidate(ctx context.Context, clipID, id int64) *Candidate {
	c, err := scanCandidate(db.Q().QueryRow(ctx, `SELECT `+candidateColumns+` FROM audio_clip_candidates
		WHERE audio_clip_id = $1 AND id = $2 LIMIT 1`, clipID, id))
	must(err)
	return c
}

func (c *Candidate) storage() *audio.Storage {
	lang := c.Language
	return audio.NewStorage(audio.Build(c.Provider, &lang))
}

// advisoryLock ports Audio::GenerationLock's key.
func advisoryLock(key string) int64 {
	sum := sha256.Sum256([]byte("audio-clip:" + key))
	n, _ := strconv.ParseInt(hex.EncodeToString(sum[:])[:15], 16, 64)
	return n
}

// ArgumentError is the Ruby ArgumentError the acceptor raises.
type ArgumentError struct{ Message string }

func (e *ArgumentError) Error() string     { return e.Message }
func (e *ArgumentError) RubyClass() string { return "ArgumentError" }

// AcceptCandidate ports Audio::CandidateAcceptor#call: the candidate's file
// becomes the clip for its key, usages move over, a customization it carried
// becomes active and the clip's other pending candidates are rejected.
func AcceptCandidate(ctx context.Context, cand *Candidate) (*Clip, error) {
	storage := cand.storage()
	var replacement *Clip
	err := db.Transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		locked, err := scanCandidate(tx.QueryRow(ctx, `SELECT `+candidateColumns+` FROM audio_clip_candidates WHERE id = $1 FOR UPDATE`, cand.ID))
		if err != nil {
			return err
		}
		if locked == nil || locked.Status != "pending" {
			return &ArgumentError{"audio candidate is no longer pending"}
		}
		cand = locked
		lock := advisoryLock(cand.Key)
		// GenerationLock: held for the rest of this transaction (Rails holds
		// the session lock for the same span).
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, lock); err != nil {
			return err
		}
		destination := storage.FilenameFor(cand.Key)
		if err := storage.Copy(ctx, cand.Filename, destination); err != nil {
			return err
		}
		source := FindClip(ctx, cand.AudioClipID)
		now := clock.Now().UTC().Truncate(time.Microsecond)
		var id int64
		err = tx.QueryRow(ctx, `INSERT INTO audio_clips (key, filename, text, line_type, kind, provider, voice, model, speed, duration,
			character_count, language, instructions_sha256, configuration_fingerprint, custom_instructions_sha256, created_at, updated_at)
			VALUES ($1, $2, $3, $4, 'line', $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $15)
			ON CONFLICT (key) DO UPDATE SET filename = EXCLUDED.filename, text = EXCLUDED.text, line_type = EXCLUDED.line_type,
			  kind = EXCLUDED.kind, provider = EXCLUDED.provider, voice = EXCLUDED.voice, model = EXCLUDED.model, speed = EXCLUDED.speed,
			  duration = EXCLUDED.duration, character_count = EXCLUDED.character_count, language = EXCLUDED.language,
			  instructions_sha256 = EXCLUDED.instructions_sha256, configuration_fingerprint = EXCLUDED.configuration_fingerprint,
			  custom_instructions_sha256 = EXCLUDED.custom_instructions_sha256,
			  updated_at = CASE WHEN (audio_clips.filename, audio_clips.text, audio_clips.line_type, audio_clips.kind, audio_clips.provider,
			    audio_clips.voice, audio_clips.model, audio_clips.speed, audio_clips.duration, audio_clips.character_count,
			    audio_clips.language, audio_clips.instructions_sha256, audio_clips.configuration_fingerprint,
			    audio_clips.custom_instructions_sha256) IS DISTINCT FROM (EXCLUDED.filename, EXCLUDED.text, EXCLUDED.line_type,
			    EXCLUDED.kind, EXCLUDED.provider, EXCLUDED.voice, EXCLUDED.model, EXCLUDED.speed, EXCLUDED.duration,
			    EXCLUDED.character_count, EXCLUDED.language, EXCLUDED.instructions_sha256, EXCLUDED.configuration_fingerprint,
			    EXCLUDED.custom_instructions_sha256) THEN EXCLUDED.updated_at ELSE audio_clips.updated_at END
			RETURNING id`,
			cand.Key, destination, cand.Text, cand.LineType, cand.Provider, cand.Voice, cand.Model, cand.Speed, cand.Duration,
			cand.CharacterCount, cand.Language, cand.InstructionsSHA, cand.Fingerprint, cand.CustomInstructionsSHA, now).Scan(&id)
		if err != nil {
			return err
		}
		if source != nil && id != source.ID {
			// transfer_usages
			if _, err := tx.Exec(ctx, `INSERT INTO audio_clip_usages (audio_clip_id, prayer_book_code, source_name, source_key, created_at, updated_at)
				SELECT $2, u.prayer_book_code, u.source_name, u.source_key, $3, $3 FROM audio_clip_usages u
				WHERE u.audio_clip_id = $1 AND NOT EXISTS (SELECT 1 FROM audio_clip_usages x WHERE x.audio_clip_id = $2
				  AND x.prayer_book_code = u.prayer_book_code AND x.source_name = u.source_name AND x.source_key = u.source_key)
				ORDER BY u.id`, source.ID, id, now); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `DELETE FROM audio_clip_usages WHERE audio_clip_id = $1`, source.ID); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `UPDATE audio_clip_candidates SET audio_clip_id = $2, updated_at = $3 WHERE id = $1`, cand.ID, id, now); err != nil {
				return err
			}
			cand.AudioClipID = id
			// remove_source
			if source.Filename != destination {
				if err := audio.PlainStorage().Delete(ctx, source.Filename); err != nil {
					return err
				}
			}
			for _, t := range []string{"audio_clip_usages", "user_audio_usages", "audio_clip_candidates"} {
				if _, err := tx.Exec(ctx, `DELETE FROM `+t+` WHERE audio_clip_id = $1`, source.ID); err != nil {
					return err
				}
			}
			if _, err := tx.Exec(ctx, `DELETE FROM audio_clips WHERE id = $1`, source.ID); err != nil {
				return err
			}
		}
		if cand.Filename != destination {
			if err := storage.Delete(ctx, cand.Filename); err != nil {
				return err
			}
		}
		// activate_customization
		if cand.CustomInstructions != nil && !rb.BlankString(*cand.CustomInstructions) {
			digest := textDigest(cand.Text)
			var cid int64
			err := tx.QueryRow(ctx, `SELECT id FROM audio_clip_customizations WHERE text_digest = $1 AND provider = $2 AND model = $3
				AND voice = $4 AND language = $5 LIMIT 1`, digest, cand.Provider, cand.Model, cand.Voice, cand.Language).Scan(&cid)
			switch {
			case db.NoRows(err):
				_, err = tx.Exec(ctx, `INSERT INTO audio_clip_customizations (text_digest, provider, model, voice, language, text,
					instructions, instructions_sha256, status, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'active', $9, $9)`,
					digest, cand.Provider, cand.Model, cand.Voice, cand.Language, cand.Text, *cand.CustomInstructions, cand.CustomInstructionsSHA, now)
			case err == nil:
				_, err = tx.Exec(ctx, `UPDATE audio_clip_customizations SET text = $2::text, instructions = $3::text, instructions_sha256 = $4::varchar,
					status = 'active', updated_at = CASE WHEN (text, instructions, instructions_sha256, status) IS DISTINCT FROM ($2::text, $3::text, $4::varchar, 'active'::varchar)
					THEN $5 ELSE updated_at END WHERE id = $1`, cid, cand.Text, *cand.CustomInstructions, cand.CustomInstructionsSHA, now)
			}
			if err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, `UPDATE audio_clip_candidates SET status = 'accepted', updated_at = $2 WHERE id = $1`, cand.ID, now); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE audio_clip_candidates SET status = 'rejected', updated_at = $3
			WHERE audio_clip_id = $1 AND status = 'pending' AND id <> $2`, cand.AudioClipID, cand.ID, clock.Now().UTC()); err != nil {
			return err
		}
		replacement = FindClip(ctx, id)
		return nil
	})
	return replacement, err
}

// RejectCandidate ports Audio::CandidateRejector#call.
func RejectCandidate(ctx context.Context, cand *Candidate) error {
	storage := cand.storage()
	return db.Transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		locked, err := scanCandidate(tx.QueryRow(ctx, `SELECT `+candidateColumns+` FROM audio_clip_candidates WHERE id = $1 FOR UPDATE`, cand.ID))
		if err != nil || locked == nil || locked.Status != "pending" {
			return err
		}
		if err := storage.Delete(ctx, locked.Filename); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE audio_clip_candidates SET status = 'rejected', updated_at = $2 WHERE id = $1`, locked.ID, clock.Now().UTC())
		return err
	})
}
