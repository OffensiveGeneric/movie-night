package library

import (
	"regexp"
	"strconv"
	"strings"
)

var (
	// "Star.Trek.DS9-s01e12.Battle.Lines" or "Show 1x05 Title"
	sxxeyy = regexp.MustCompile(`(?i)^(.*?)[\s._-]*\bs(\d{1,2})[\s._-]?e(\d{1,3})\b(.*)$`)
	nxnn   = regexp.MustCompile(`(?i)^(.*?)[\s._-]*\b(\d{1,2})x(\d{2,3})\b(.*)$`)

	// Release-site prefixes like "www.Example.com - " or "[ www.Example.com ] - ".
	sitePrefix = regexp.MustCompile(`(?i)^\[?\s*www\.[^\s\]]+\s*\]?\s*-?\s*`)
	bracketed  = regexp.MustCompile(`\[[^\]]*\]|\{[^}]*\}`)
	openParen  = regexp.MustCompile(`\s*\([^)]*$`) // "(Drama " left dangling before a year
	parenYear  = regexp.MustCompile(`\((19\d{2}|20\d{2})\)`)
	bareYear   = regexp.MustCompile(`\b(19\d{2}|20\d{2})\b`)
	spaces     = regexp.MustCompile(`\s+`)
)

// clean turns a dotted release name into readable words.
func clean(s string) string {
	s = sitePrefix.ReplaceAllString(s, "")
	s = bracketed.ReplaceAllString(s, " ")
	s = openParen.ReplaceAllString(s, "")
	s = strings.NewReplacer(".", " ", "_", " ").Replace(s)
	s = spaces.ReplaceAllString(s, " ")
	return strings.Trim(s, " -([")
}

type parsedEpisode struct {
	show            string
	season, episode int
	title           string
}

// parseEpisode recognises a season/episode code in a file name (without
// extension). ok is false when there is none.
func parseEpisode(name string) (p parsedEpisode, ok bool) {
	m := sxxeyy.FindStringSubmatch(name)
	if m == nil {
		m = nxnn.FindStringSubmatch(name)
	}
	if m == nil {
		return p, false
	}
	p.show = clean(m[1])
	p.season, _ = strconv.Atoi(m[2])
	p.episode, _ = strconv.Atoi(m[3])
	p.title = clean(m[4])
	return p, true
}

// parseMovie pulls a title and year out of a file or folder name. The year
// is 0 when none is found. A year in parentheses wins over a bare one, so
// "Blade Runner 2049 (2017)" parses correctly; a bare year at the very start
// is treated as part of the title ("2001 A Space Odyssey").
func parseMovie(name string) (title string, year int) {
	name = sitePrefix.ReplaceAllString(name, "")
	if loc := parenYear.FindStringSubmatchIndex(name); loc != nil && loc[0] > 0 {
		year, _ = strconv.Atoi(name[loc[2]:loc[3]])
		return clean(name[:loc[0]]), year
	}
	for _, loc := range bareYear.FindAllStringSubmatchIndex(name, -1) {
		if loc[0] == 0 {
			continue
		}
		year, _ = strconv.Atoi(name[loc[2]:loc[3]])
		return clean(name[:loc[0]]), year
	}
	return clean(name), 0
}
