package models

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
)

// StringSlice is a []string that serialises as a JSON array in a SQLite TEXT column.
type StringSlice []string

func (s StringSlice) Value() (driver.Value, error) {
	b, err := json.Marshal(s)
	return string(b), err
}

func (s *StringSlice) Scan(src any) error {
	switch v := src.(type) {
	case string:
		return json.Unmarshal([]byte(v), s)
	case []byte:
		return json.Unmarshal(v, s)
	case nil:
		*s = StringSlice{}
		return nil
	default:
		return fmt.Errorf("StringSlice: cannot scan %T", src)
	}
}

// CampaignPlatform is a target platform entry on a campaign, carrying the
// platform ID and the subset of post types selected for that campaign.
type CampaignPlatform struct {
	ID        string   `json:"id"`
	PostTypes []string `json:"post_types"`
}

// CampaignPlatforms is a []CampaignPlatform that serialises as a JSON array
// in a SQLite TEXT column.
type CampaignPlatforms []CampaignPlatform

func (s CampaignPlatforms) Value() (driver.Value, error) {
	b, err := json.Marshal(s)
	return string(b), err
}

func (s *CampaignPlatforms) Scan(src any) error {
	switch v := src.(type) {
	case string:
		return json.Unmarshal([]byte(v), s)
	case []byte:
		return json.Unmarshal(v, s)
	case nil:
		*s = CampaignPlatforms{}
		return nil
	default:
		return fmt.Errorf("CampaignPlatforms: cannot scan %T", src)
	}
}

// PostTypeMap is a map[string]string that serialises as a JSON object in a
// SQLite TEXT column. Keys are post-type slugs, values are display names.
type PostTypeMap map[string]string

func (m PostTypeMap) Value() (driver.Value, error) {
	b, err := json.Marshal(m)
	return string(b), err
}

func (m *PostTypeMap) Scan(src any) error {
	switch v := src.(type) {
	case string:
		return json.Unmarshal([]byte(v), m)
	case []byte:
		return json.Unmarshal(v, m)
	case nil:
		*m = PostTypeMap{}
		return nil
	default:
		return fmt.Errorf("PostTypeMap: cannot scan %T", src)
	}
}
