package rosary

import (
	"context"
	"github.com/dodopok/estevao-api-go/internal/ar"
	"time"

	"github.com/dodopok/estevao-api-go/internal/db"
	"github.com/dodopok/estevao-api-go/internal/rb"
)

// Model constants (CustomRosaryPrayer, CustomRosaryBlock, CustomRosaryStep).
const (
	MaxBlocks                       = 40
	MaxExpandedSteps                = 500
	MaxPerUser                      = 300
	MaxAutomaticPublicationAttempts = 10
	MaxStepsPerBlock                = 40
)

var (
	Locales             = []string{"pt-BR", "pt-PT", "en", "es"}
	ShareStatuses       = []string{"private", "pending_review", "approved", "rejected"}
	ModerationDecisions = []string{"approved", "rejected"}
	PublicationStatuses = []string{"pending", "publishing", "published", "failed", "unpublishing", "unpublished"}
	StepTypes           = []string{"cross", "invitatory", "cruciform", "week", "dismissal", "reflection"}
	contentAttributes   = []string{"title", "description", "locale", "cycle_repeat"}
)

var prayerSchema = &ar.Schema{
	Table: "custom_rosary_prayers",
	Columns: []string{"client_id", "created_at", "cycle_repeat", "description", "is_public", "last_moderation_reentry_at",
		"locale", "moderation_decision", "moderation_note", "moderation_reentry_count", "publication_attempts",
		"publication_category", "publication_error", "publication_locale", "publication_retry_at", "publication_started_at",
		"publication_status", "published_at", "reviewed_at", "share_status", "source_hash", "source_revision",
		"strapi_document_id", "strapi_slug", "title", "updated_at", "user_id"},
	Types: map[string]ar.ColType{
		"client_id": ar.String, "created_at": ar.Datetime, "cycle_repeat": ar.Integer, "description": ar.String, "is_public": ar.Boolean,
		"last_moderation_reentry_at": ar.Datetime, "locale": ar.String, "moderation_decision": ar.String, "moderation_note": ar.String,
		"moderation_reentry_count": ar.Integer, "publication_attempts": ar.Integer, "publication_category": ar.JSON,
		"publication_error": ar.String, "publication_locale": ar.String, "publication_retry_at": ar.Datetime,
		"publication_started_at": ar.Datetime, "publication_status": ar.String, "published_at": ar.Datetime, "reviewed_at": ar.Datetime,
		"share_status": ar.String, "source_hash": ar.String, "source_revision": ar.String, "strapi_document_id": ar.String,
		"strapi_slug": ar.String, "title": ar.String, "updated_at": ar.Datetime, "user_id": ar.Integer,
	},
	Defaults: map[string]any{"cycle_repeat": int64(1), "is_public": false, "locale": "pt-BR", "moderation_reentry_count": int64(0),
		"publication_attempts": int64(0), "publication_status": "pending", "share_status": "private"},
}

var blockSchema = &ar.Schema{
	Table:   "custom_rosary_blocks",
	Columns: []string{"client_id", "created_at", "custom_rosary_prayer_id", "in_cycle", "name", "position", "repeat_count", "updated_at"},
	Types: map[string]ar.ColType{"client_id": ar.String, "created_at": ar.Datetime, "custom_rosary_prayer_id": ar.Integer, "in_cycle": ar.Boolean,
		"name": ar.String, "position": ar.Integer, "repeat_count": ar.Integer, "updated_at": ar.Datetime},
	Defaults: map[string]any{"in_cycle": false, "repeat_count": int64(1)},
}

var stepSchema = &ar.Schema{
	Table: "custom_rosary_steps",
	Columns: []string{"client_id", "created_at", "custom_rosary_block_id", "position", "repeat_count", "step_type", "text", "title",
		"updated_at"},
	Types: map[string]ar.ColType{"client_id": ar.String, "created_at": ar.Datetime, "custom_rosary_block_id": ar.Integer, "position": ar.Integer,
		"repeat_count": ar.Integer, "step_type": ar.String, "text": ar.String, "title": ar.String, "updated_at": ar.Datetime},
	Defaults: map[string]any{"repeat_count": int64(1)},
}

// Prayer is a CustomRosaryPrayer with its blocks.
type Prayer struct {
	*ar.Record
	Blocks []*Block
}

// Block is a CustomRosaryBlock with its steps.
type Block struct {
	*ar.Record
	Steps []*Step
}

// Step is a CustomRosaryStep.
type Step struct{ *ar.Record }

// UserID is the owner.
func (p *Prayer) UserID() int64 { return p.Int("user_id") }

// NewPrayer ports user.custom_rosary_prayers.new.
func NewPrayer(userID int64) *Prayer {
	p := &Prayer{Record: ar.NewRecord(prayerSchema)}
	p.Set("user_id", userID)
	return p
}

// LoadWithStructure ports CustomRosaryPrayer.with_structure.where(where):
// the prayers in the given order, their blocks and steps preloaded by
// position.
func LoadWithStructure(ctx context.Context, q db.Querier, where, order string, args ...any) ([]*Prayer, error) {
	sql := "SELECT " + prayerSchema.SelectList("custom_rosary_prayers") + " FROM custom_rosary_prayers"
	if where != "" {
		sql += " WHERE " + where
	}
	if order != "" {
		sql += " ORDER BY " + order
	}
	rows, err := q.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	var prayers []*Prayer
	byID := map[int64]*Prayer{}
	var ids []int64
	for rows.Next() {
		r, err := ar.ScanRecord(prayerSchema, rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		p := &Prayer{Record: r}
		prayers = append(prayers, p)
		byID[p.ID] = p
		ids = append(ids, p.ID)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return prayers, nil
	}
	rows, err = q.Query(ctx, "SELECT "+blockSchema.SelectList("custom_rosary_blocks")+
		" FROM custom_rosary_blocks WHERE custom_rosary_blocks.custom_rosary_prayer_id = ANY($1) ORDER BY custom_rosary_blocks.position ASC", ids)
	if err != nil {
		return nil, err
	}
	blockByID := map[int64]*Block{}
	var blockIDs []int64
	for rows.Next() {
		r, err := ar.ScanRecord(blockSchema, rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		b := &Block{Record: r}
		p := byID[b.Int("custom_rosary_prayer_id")]
		p.Blocks = append(p.Blocks, b)
		blockByID[b.ID] = b
		blockIDs = append(blockIDs, b.ID)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(blockIDs) == 0 {
		return prayers, nil
	}
	rows, err = q.Query(ctx, "SELECT "+stepSchema.SelectList("custom_rosary_steps")+
		" FROM custom_rosary_steps WHERE custom_rosary_steps.custom_rosary_block_id = ANY($1) ORDER BY custom_rosary_steps.position ASC", blockIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		r, err := ar.ScanRecord(stepSchema, rows)
		if err != nil {
			return nil, err
		}
		b := blockByID[r.Int("custom_rosary_block_id")]
		b.Steps = append(b.Steps, &Step{r})
	}
	return prayers, rows.Err()
}

// Find ports CustomRosaryPrayer.with_structure.find_by(id:) (nil if none).
func Find(ctx context.Context, q db.Querier, id int64) (*Prayer, error) {
	list, err := LoadWithStructure(ctx, q, "custom_rosary_prayers.ID = $1", "", id)
	if err != nil || len(list) == 0 {
		return nil, err
	}
	return list[0], nil
}

func timeJSON(v any) any {
	if t, ok := v.(time.Time); ok {
		return rb.FormatTime(t)
	}
	return nil
}

// JSON ports CustomRosaryPrayerSerializer#as_json.
func (p *Prayer) JSON() *rb.Map {
	blocks := []any{}
	for _, b := range p.Blocks {
		steps := []any{}
		for _, s := range b.Steps {
			steps = append(steps, rb.M("id", s.ID, "client_id", s.Get("client_id"), "position", s.Get("position"),
				"step_type", s.Get("step_type"), "title", s.Get("title"), "text", s.Get("text"), "repeat_count", s.Get("repeat_count")))
		}
		blocks = append(blocks, rb.M("id", b.ID, "client_id", b.Get("client_id"), "position", b.Get("position"), "name", b.Get("name"),
			"repeat_count", b.Get("repeat_count"), "in_cycle", b.Get("in_cycle"), "steps", steps))
	}
	return rb.M("id", p.ID, "client_id", p.Get("client_id"), "title", p.Get("title"), "description", p.Get("description"),
		"locale", p.Get("locale"), "cycle_repeat", p.Get("cycle_repeat"), "is_public", p.Get("is_public"),
		"share_status", p.Get("share_status"), "created_at", timeJSON(p.Get("created_at")), "updated_at", timeJSON(p.Get("updated_at")),
		"blocks", blocks)
}
