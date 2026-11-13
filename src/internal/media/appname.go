package media

import (
	"regexp"
	"strings"
	"unicode"
)

var knownApps = map[string]string{
	"chrome":    "Chrome",
	"msedge":    "Edge",
	"firefox":   "Firefox",
	"spotify":   "Spotify",
	"vlc":       "VLC",
	"brave":     "Brave",
	"opera":     "Opera",
	"zunemusic": "Media Player",
	// Firefox registers its sessions under this fixed install hash, not its name.
	"308046b0af4a39cb": "Firefox",
}

var opaqueID = regexp.MustCompile(`^[0-9a-fA-F]{12,}$`)

// AppName turns a Windows app user model id into something worth showing, or
// "" when the id carries no readable name.
func AppName(appID string) string {
	name := appID
	// Packaged apps look like "Publisher.Package_hash!AppName".
	if i := strings.LastIndex(name, "!"); i >= 0 {
		name = name[i+1:]
	}
	name = strings.TrimSuffix(strings.TrimSuffix(name, ".exe"), ".EXE")
	if known, ok := knownApps[strings.ToLower(name)]; ok {
		return known
	}
	if name == "" || opaqueID.MatchString(name) {
		return ""
	}
	runes := []rune(name)
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}
