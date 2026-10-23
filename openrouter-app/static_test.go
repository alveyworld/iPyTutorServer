package main

import (
	"regexp"
	"testing"
)

// The tutor can be served under a path on another site, such as intebg.org/tutor/ next
// to the interview app. The page must then ask for its files and API relative to itself:
// an absolute path ("/api/chat") would go to the other app at the site's root.
func TestPageUsesRelativePaths(t *testing.T) {
	absolute := regexp.MustCompile(`(?:src|href)="/[^/]|fetch\(\s*["'\x60]/[^/]`)
	for _, name := range []string{"index.html", "app.js"} {
		data, err := staticFiles.ReadFile("static/" + name)
		if err != nil {
			t.Fatal(err)
		}
		for _, loc := range absolute.FindAllIndex(data, -1) {
			end := min(loc[1]+30, len(data))
			t.Errorf("static/%s uses an absolute path: %q", name, data[loc[0]:end])
		}
	}
}
