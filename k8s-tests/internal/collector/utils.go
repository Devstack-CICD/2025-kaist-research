package collector

import (
	"regexp"
)

var sanitizer = regexp.MustCompile(`[^A-Za-z0-9_-]`)

func safeID(ns, name string) string {
	return sanitizer.ReplaceAllString(ns+"_"+name, "_")

}

