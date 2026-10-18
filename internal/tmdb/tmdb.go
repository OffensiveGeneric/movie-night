// Package tmdb is a minimal client for the parts of The Movie Database API
// that Movie Night uses: search, season details and images.
//
// This product uses the TMDB API but is not endorsed or certified by TMDB.
package tmdb

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Client struct {
	Key     string // v3 API key or v4 read access token
	BaseURL string // default https://api.themoviedb.org/3
	ImgURL  string // default https://image.tmdb.org/t/p
	HTTP    *http.Client
}

func New(key string) *Client {
	return &Client{
		Key:     key,
		BaseURL: "https://api.themoviedb.org/3",
		ImgURL:  "https://image.tmdb.org/t/p",
		HTTP:    &http.Client{Timeout: 20 * time.Second},
	}
}

type Result struct {
	ID         int    `json:"id"`
	PosterPath string `json:"poster_path"`
}

type Episode struct {
	EpisodeNumber int    `json:"episode_number"`
	StillPath     string `json:"still_path"`
}

func (c *Client) get(ctx context.Context, path string, q url.Values, out any) error {
	if q == nil {
		q = url.Values{}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "", nil)
	if err != nil {
		return err
	}
	// A v4 read access token is a JWT and goes in a header; a v3 key is a
	// query parameter.
	if strings.HasPrefix(c.Key, "eyJ") {
		req.Header.Set("Authorization", "Bearer "+c.Key)
	} else {
		q.Set("api_key", c.Key)
	}
	req.URL, err = url.Parse(c.BaseURL + path + "?" + q.Encode())
	if err != nil {
		return err
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("tmdb %s: %s", path, res.Status)
	}
	return json.NewDecoder(res.Body).Decode(out)
}

// Search returns the best match for a title, or nil. kind is "tv" or "movie".
// A year narrows the search; if nothing matches with it, the search is
// retried without.
func (c *Client) Search(ctx context.Context, kind, title string, year int) (*Result, error) {
	yearParam := "year"
	if kind == "tv" {
		yearParam = "first_air_date_year"
	}
	for _, withYear := range []bool{year > 0, false} {
		q := url.Values{"query": {title}, "include_adult": {"false"}}
		if withYear {
			q.Set(yearParam, strconv.Itoa(year))
		}
		var res struct {
			Results []Result `json:"results"`
		}
		if err := c.get(ctx, "/search/"+kind, q, &res); err != nil {
			return nil, err
		}
		if len(res.Results) > 0 {
			return &res.Results[0], nil
		}
		if !withYear {
			break
		}
	}
	return nil, nil
}

// Details fetches a movie or show directly by ID (for pinned overrides).
func (c *Client) Details(ctx context.Context, kind string, id int) (*Result, error) {
	r := &Result{}
	return r, c.get(ctx, "/"+kind+"/"+strconv.Itoa(id), nil, r)
}

// Season lists a season's episodes with their stills.
func (c *Client) Season(ctx context.Context, showID, season int) ([]Episode, error) {
	var res struct {
		Episodes []Episode `json:"episodes"`
	}
	err := c.get(ctx, fmt.Sprintf("/tv/%d/season/%d", showID, season), nil, &res)
	return res.Episodes, err
}

// Image downloads an image at the given size ("w342", "w300", ...).
func (c *Client) Image(ctx context.Context, size, path string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.ImgURL+"/"+size+path, nil)
	if err != nil {
		return nil, err
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tmdb image %s: %s", path, res.Status)
	}
	return io.ReadAll(io.LimitReader(res.Body, 10<<20))
}
