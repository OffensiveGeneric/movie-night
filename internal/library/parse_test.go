package library

import "testing"

func TestParseEpisode(t *testing.T) {
	tests := []struct {
		in   string
		want parsedEpisode
	}{
		{"Star.Trek.DS9-s01e12.Battle.Lines", parsedEpisode{"Star Trek DS9", 1, 12, "Battle Lines"}},
		{"Star.Trek.DS9-s07e25.What.You.Leave.Behind", parsedEpisode{"Star Trek DS9", 7, 25, "What You Leave Behind"}},
		{"Show Me A Hero S01E03", parsedEpisode{"Show Me A Hero", 1, 3, ""}},
		{"The.Wire.1x05.The.Pager", parsedEpisode{"The Wire", 1, 5, "The Pager"}},
	}
	for _, tt := range tests {
		got, ok := parseEpisode(tt.in)
		if !ok || got != tt.want {
			t.Errorf("parseEpisode(%q) = %+v, %v; want %+v", tt.in, got, ok, tt.want)
		}
	}
	if p, ok := parseEpisode("Baseball (1994) Inning 1"); ok {
		t.Errorf("parseEpisode found an episode code in a name without one: %+v", p)
	}
}

func TestParseMovie(t *testing.T) {
	tests := []struct {
		in    string
		title string
		year  int
	}{
		{"Office Space (1999) (1080p BluRay x265 HEVC 10bit AAC 5.1 Bandi)", "Office Space", 1999},
		{"Harold.and.Maude.1971.1080p.Criterion.Bluray.DTS.x264-GCJM", "Harold and Maude", 1971},
		{"www.Torrenting.com   -    Fort Apache the Bronx 1981 DVDRip", "Fort Apache the Bronx", 1981},
		{"[ www.TorrentDay.com ] - The Ruling Class (1972) - [DVDRip]", "The Ruling Class", 1972},
		{"Charlie Chaplin - The Gold Rush (1925)", "Charlie Chaplin - The Gold Rush", 1925},
		{"A  Face in the Crowd  (Drama 1957)  Andy Griffith  720p", "A Face in the Crowd", 1957},
		{"Blade Runner 2049 (2017) [2160p]", "Blade Runner 2049", 2017},
		{"2001 A Space Odyssey", "2001 A Space Odyssey", 0},
		{"La Dolce Vita ", "La Dolce Vita", 0},
	}
	for _, tt := range tests {
		title, year := parseMovie(tt.in)
		if title != tt.title || year != tt.year {
			t.Errorf("parseMovie(%q) = %q, %d; want %q, %d", tt.in, title, year, tt.title, tt.year)
		}
	}
}
