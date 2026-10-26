package httpx

import "testing"

func TestPaginationClampsAndDefaults(t *testing.T) {
	page, size := 3, 500
	sort := "name"
	p := Paginate(&page, &size, Deref(&sort), "DESC")
	if p.Page != 3 || p.Size != 100 || p.Sort != "name" || p.Direction != "desc" || p.Offset() != 200 {
		t.Fatalf("%+v", p)
	}
	page, size = -1, 0
	p = Paginate(&page, &size, Deref[string](nil), "")
	if p.Page != 1 || p.Size != 25 || p.Sort != "" || p.Direction != "asc" {
		t.Fatalf("defaults: %+v", p)
	}
	if m := p.Meta(51); m.Pages != 3 || m.Total != 51 || m.Size != 25 {
		t.Fatalf("meta: %+v", m)
	}
}

func TestIfMatchReadsEveryETagShape(t *testing.T) {
	for in, want := range map[string]int{`"7"`: 7, `W/"7"`: 7, `7`: 7, ` "12" `: 12} {
		if v, ok := IfMatch(in); !ok || v != want {
			t.Errorf("%q = %d %v", in, v, ok)
		}
	}
	for _, in := range []string{"", `"abc"`, `*`, `"1", "2"`} {
		if _, ok := IfMatch(in); ok {
			t.Errorf("%q accepted", in)
		}
	}
	if ETag(3) != `"3"` {
		t.Fatal(ETag(3))
	}
}
