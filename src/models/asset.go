package models

import (
	"time"

	"github.com/uptrace/bun"
)

const (
	AssetStatusPending    = "pending"
	AssetStatusProcessing = "processing"
	AssetStatusReady      = "ready"
	AssetStatusPartial    = "partial"
	AssetStatusFailed     = "failed"
)

const (
	// AssetTypeMarkdown marks assets imported from a `.md` upload.
	AssetTypeMarkdown = "MD"
	// AssetTypePDF marks assets imported from a `.pdf` upload.
	AssetTypePDF = "PDF"
	// AssetTypeImage marks assets produced by image generation (CON-105).
	AssetTypeImage = "IMG"
)

type Asset struct {
	bun.BaseModel `bun:"table:assets,alias:a" swaggerignore:"true"`
	TenantScoped  // CON-97: tenant_id column + central scoping hooks

	ID          string           `bun:"id,pk"                                        json:"id"`
	Title       string           `bun:"title,notnull"                                json:"title"`
	Content     string           `bun:"content,notnull"                              json:"content"`
	Status      string           `bun:"status,notnull,default:'ready'"               json:"status"`
	Type        *string          `bun:"type"                                         json:"type"`
	AIGenerated bool             `bun:"ai_generated,notnull,default:false"           json:"ai_generated"`
	BrandStyle  bool             `bun:"brand_style,notnull,default:false"            json:"brand_style"`
	Generation  *AssetGeneration `bun:"generation,type:jsonb"                        json:"generation,omitempty"`
	TagIDs      StringSlice      `bun:"tag_ids,notnull,type:jsonb"                   json:"tag_ids"`
	Tags        []Tag            `bun:"-"                                            json:"tags"`
	File        *AssetFile       `bun:"-"                                            json:"file,omitempty"`
	CreatedBy   string           `bun:"created_by,notnull"                           json:"created_by"`
	CreatedAt   time.Time        `bun:"created_at,notnull,default:current_timestamp" json:"created_at"`
	UpdatedAt   time.Time        `bun:"updated_at,notnull,default:current_timestamp" json:"updated_at"`
}

// AssetGeneration is the provenance + reproducibility metadata persisted on an
// AI-generated asset (CON-105 §5/§7/§10), stored in the assets.generation jsonb
// column. It is nil for non-generated assets. The authoritative output size
// (§7) and the model used are captured so a generation can be reproduced; the
// SynthID / C2PA flags back the AI-generated disclosure surface (§10). The cost
// snapshot is recorded separately in usage_events (§9), not here.
type AssetGeneration struct {
	Model       string `json:"model"`              // e.g. "gemini-3.1-flash-image"
	AspectRatio string `json:"aspect_ratio"`       // e.g. "4:5"
	Resolution  string `json:"resolution"`         // e.g. "1K"
	SynthID     bool   `json:"synth_id,omitempty"` // SynthID watermark present
	C2PA        bool   `json:"c2pa,omitempty"`     // C2PA content credentials present
}
