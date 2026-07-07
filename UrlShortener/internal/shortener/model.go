package shortener

import "time"

const (
	LinkKindURL    = "url"
	LinkKindObject = "object"
)

type Link struct {
	Code      string     `json:"code" bson:"code"`
	Kind      string     `json:"kind" bson:"kind"`
	URL       string     `json:"url" bson:"url"`
	Title     string     `json:"title,omitempty" bson:"title,omitempty"`
	Scope     string     `json:"scope,omitempty" bson:"scope,omitempty"`
	Visits    int64      `json:"visits" bson:"visits"`
	MaxVisits int64      `json:"max_visits,omitempty" bson:"max_visits,omitempty"`
	CreatedAt time.Time  `json:"created_at" bson:"created_at"`
	UpdatedAt time.Time  `json:"updated_at" bson:"updated_at"`
	IsDeleted bool       `json:"is_deleted" bson:"is_deleted"`
	ExpiresAt *time.Time `json:"expires_at,omitempty" bson:"expires_at,omitempty"`

	ObjectBucket      string `json:"object_bucket,omitempty" bson:"object_bucket,omitempty"`
	ObjectKey         string `json:"object_key,omitempty" bson:"object_key,omitempty"`
	ObjectFileName    string `json:"object_file_name,omitempty" bson:"object_file_name,omitempty"`
	ObjectContentType string `json:"object_content_type,omitempty" bson:"object_content_type,omitempty"`
	ObjectSize        int64  `json:"object_size,omitempty" bson:"object_size,omitempty"`
}

func (l Link) Expired(now time.Time) bool {
	return l.ExpiresAt != nil && !l.ExpiresAt.After(now)
}

func (l Link) VisitsExhausted() bool {
	return l.MaxVisits > 0 && l.Visits >= l.MaxVisits
}

type CreateLinkRequest struct {
	URL        string `json:"url"`
	Title      string `json:"title,omitempty"`
	Scope      string `json:"scope,omitempty"`
	TTLSeconds int64  `json:"ttl_seconds,omitempty"`
	MaxVisits  int64  `json:"max_visits,omitempty"`
}

type CreateLinkResponse struct {
	Code      string     `json:"code"`
	ShortURL  string     `json:"short_url"`
	URL       string     `json:"url"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

type CreateObjectLinkResponse struct {
	Code        string     `json:"code"`
	ShortURL    string     `json:"short_url"`
	Bucket      string     `json:"bucket"`
	ObjectKey   string     `json:"object_key"`
	FileName    string     `json:"file_name"`
	ContentType string     `json:"content_type"`
	Size        int64      `json:"size"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
}
