package httpx

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strconv"
	"time"
)

// List is the universal list envelope (spec §"列表响应 envelope").
type List[T any] struct {
	Items      []T     `json:"items"`
	NextCursor *string `json:"nextCursor"`
	HasMore    bool    `json:"hasMore"`
}

// Page holds parsed cursor-pagination parameters (spec §"分页与列表约定").
type Page struct {
	Limit  int
	Order  string // "asc" | "desc"
	Cursor *Cursor
}

// Cursor is the opaque keyset cursor, encoded server-side over (created_at, id).
type Cursor struct {
	CreatedAt time.Time `json:"c"`
	ID        string    `json:"i"`
}

// ParsePage reads limit/cursor/order from the query string with safe defaults.
func ParsePage(r *http.Request) (Page, error) {
	p := Page{Limit: 20, Order: "desc"}
	q := r.URL.Query()
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			return p, errBadLimit
		}
		if n > 100 {
			n = 100
		}
		p.Limit = n
	}
	if v := q.Get("order"); v == "asc" || v == "desc" {
		p.Order = v
	}
	if v := q.Get("cursor"); v != "" {
		c, err := DecodeCursor(v)
		if err != nil {
			return p, errBadCursor
		}
		p.Cursor = c
	}
	return p, nil
}

var (
	errBadLimit  = &pageErr{"invalid limit"}
	errBadCursor = &pageErr{"invalid cursor"}
)

type pageErr struct{ msg string }

func (e *pageErr) Error() string { return e.msg }

// EncodeCursor encodes a keyset cursor into an opaque, caller-unparseable token.
func EncodeCursor(createdAt time.Time, id string) string {
	b, _ := json.Marshal(Cursor{CreatedAt: createdAt, ID: id})
	return base64.RawURLEncoding.EncodeToString(b)
}

// DecodeCursor parses an opaque cursor token.
func DecodeCursor(s string) (*Cursor, error) {
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return nil, err
	}
	var c Cursor
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

// NewList builds a list envelope, computing the next cursor from the last item
// when the page is full.
func NewList[T any](items []T, limit int, cursorOf func(T) (time.Time, string)) List[T] {
	out := List[T]{Items: items}
	if out.Items == nil {
		out.Items = []T{}
	}
	if len(items) == limit && limit > 0 {
		t, idv := cursorOf(items[len(items)-1])
		c := EncodeCursor(t, idv)
		out.NextCursor = &c
		out.HasMore = true
	}
	return out
}
