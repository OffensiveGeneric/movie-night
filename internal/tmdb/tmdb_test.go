package tmdb

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// The folder says 2001, TMDB says 2002, and the top hit for the bare title
// is a different show: the near-year result should win.
func TestSearchPrefersNearYearOnRetry(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		results := []map[string]any{}
		if r.URL.Query().Get("first_air_date_year") == "" {
			results = []map[string]any{
				{"id": 45595, "first_air_date": "1999-01-20"},
				{"id": 16880, "first_air_date": "2002-01-14"},
			}
		}
		json.NewEncoder(w).Encode(map[string]any{"results": results})
	}))
	defer srv.Close()

	c := New("abc")
	c.BaseURL = srv.URL
	r, err := c.Search(context.Background(), "tv", "Mark Twain", 2001)
	if err != nil || r == nil || r.ID != 16880 {
		t.Fatalf("Search = %+v, %v; want ID 16880", r, err)
	}
}
