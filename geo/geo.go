package geo

import "strings"

// Info describes metadata about a Cloudflare colo code.
type Info struct {
	Code    string
	City    string
	Country string
}

var coloCatalog = map[string]Info{
	"SJC": {Code: "SJC", City: "San Jose", Country: "US"},
	"LHR": {Code: "LHR", City: "London", Country: "GB"},
	"SIN": {Code: "SIN", City: "Singapore", Country: "SG"},
	"HKG": {Code: "HKG", City: "Hong Kong", Country: "HK"},
}

// LookupColo returns metadata for the provided colo code if known.
func LookupColo(code string) (Info, bool) {
	if code == "" {
		return Info{}, false
	}
	info, ok := coloCatalog[strings.ToUpper(code)]
	return info, ok
}
