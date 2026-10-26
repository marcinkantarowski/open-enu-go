package httpx

import (
	"math"
	"regexp"
	"strconv"
	"strings"
)

// Pagination is a list request: page and size from the query, clamped; sort
// as the spec's enum value, which the repository maps to a column in a
// switch whose default is the natural order - so an unrecognised sort is
// ignored, not rejected, and a client-supplied column name never reaches an
// ORDER BY.
type Pagination struct {
	Page      int
	Size      int
	Sort      string // "" when none or not whitelisted
	Direction string // "asc" or "desc"
}

const (
	DefaultPageSize = 25
	MaxPageSize     = 100
)

// Paginate builds the request from what the strict server parsed: a nil
// page or size means absent and takes the default; a page below 1 becomes
// 1 and a size above MaxPageSize is clamped to it. sort is kept as given
// (Deref the spec's enum into it) - the whitelist is the repository's
// switch, not this function - and direction is "asc" unless it reads
// "desc" in any case.
func Paginate(page, size *int, sort, direction string) Pagination {
	p := Pagination{Page: 1, Size: DefaultPageSize, Direction: "asc"}
	if page != nil && *page > 1 {
		p.Page = *page
	}
	if size != nil && *size > 0 {
		p.Size = min(*size, MaxPageSize)
	}
	p.Sort = sort
	if strings.EqualFold(direction, "desc") {
		p.Direction = "desc"
	}
	return p
}

// Deref is the strict server's optional enum as a plain string.
func Deref[T ~string](p *T) string {
	if p == nil {
		return ""
	}
	return string(*p)
}

// Offset is the first row of the page.
func (p Pagination) Offset() int { return (p.Page - 1) * p.Size }

// ListMeta is the `meta` half of the list envelope the ui-kit's tables read.
type ListMeta struct {
	Total int `json:"total"`
	Page  int `json:"page"`
	Size  int `json:"size"`
	Pages int `json:"pages"`
}

// Meta computes the envelope's meta for total rows.
func (p Pagination) Meta(total int) ListMeta {
	pages := 0
	if p.Size > 0 {
		pages = int(math.Ceil(float64(total) / float64(p.Size)))
	}
	return ListMeta{Total: total, Page: p.Page, Size: p.Size, Pages: pages}
}

var etagShape = regexp.MustCompile(`^(?:W/)?"?(\d+)"?$`)

// IfMatch reads the version a client holds from the If-Match header - a
// quoted ETag, weak or strong - and returns false when the client is not
// participating in locking (no header, or not a number). A missing header
// is allowed: a background job has no stale read to protect against, which
// is why the header is how a UI must write.
func IfMatch(header string) (int, bool) {
	m := etagShape.FindStringSubmatch(strings.TrimSpace(header))
	if m == nil {
		return 0, false
	}
	n, err := strconv.Atoi(m[1])
	return n, err == nil
}

// ETag formats a version as the header the client sends back.
func ETag(version int) string { return `"` + strconv.Itoa(version) + `"` }
