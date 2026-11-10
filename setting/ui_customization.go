package setting

import (
	"errors"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
)

type CustomNavigationLink struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	URL     string `json:"url"`
	Mode    string `json:"mode"`
	Header  bool   `json:"header"`
	Sidebar bool   `json:"sidebar"`
	Home    bool   `json:"home"`
}

var navigationLinkID = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)

func NormalizeTicketQuickReplies(raw string) (string, error) {
	if len(raw) > 200000 {
		return "", errors.New("invalid quick replies")
	}
	lines := make([]string, 0)
	seen := make(map[string]bool)
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || seen[line] {
			continue
		}
		if utf8.RuneCountInString(line) > 5000 || len(lines) >= 50 {
			return "", errors.New("invalid quick replies")
		}
		seen[line] = true
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n"), nil
}

func NormalizeCustomNavigationLinks(raw string) (string, error) {
	if len(raw) > 100000 {
		return "", errors.New("invalid navigation links")
	}
	links := []CustomNavigationLink{}
	if strings.TrimSpace(raw) != "" {
		if err := common.UnmarshalJsonStr(raw, &links); err != nil {
			return "", err
		}
	}
	if len(links) > 30 {
		return "", errors.New("invalid navigation links")
	}
	seen := make(map[string]bool)
	for i := range links {
		link := &links[i]
		link.Title = strings.TrimSpace(link.Title)
		link.URL = strings.TrimSpace(link.URL)
		u, err := url.Parse(link.URL)
		if !navigationLinkID.MatchString(link.ID) || seen[link.ID] || link.Title == "" || utf8.RuneCountInString(link.Title) > 80 ||
			len(link.URL) > 2048 || err != nil || u.Hostname() == "" || u.User != nil ||
			(u.Scheme != "https" && u.Scheme != "http") ||
			(link.Mode != "external" && link.Mode != "iframe") || (link.Mode == "iframe" && u.Scheme != "https") {
			return "", errors.New("invalid navigation links")
		}
		seen[link.ID] = true
	}
	if links == nil {
		links = []CustomNavigationLink{}
	}
	data, err := common.Marshal(links)
	return string(data), err
}
