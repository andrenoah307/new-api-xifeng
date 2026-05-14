package common

import (
	"regexp"
	"strings"
)

type proxyIdPattern struct {
	name  string
	regex *regexp.Regexp
}

var proxyIdPatterns = []proxyIdPattern{
	{"request_id", regexp.MustCompile(`\s*\(request id: [^)]*\)`)},
	{"request_ori_id", regexp.MustCompile(`\s*\(request_ori_id: [^)]*\)`)},
	{"traceid_fullwidth", regexp.MustCompile(`\s*\x{ff08}traceid: [^\x{ff09}]*\x{ff09}`)},
}

func StripProxyIdSuffixes(msg string) string {
	for _, p := range proxyIdPatterns {
		msg = p.regex.ReplaceAllString(msg, "")
	}
	return strings.TrimRight(msg, " ")
}
