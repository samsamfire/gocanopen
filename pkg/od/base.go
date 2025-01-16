package od

import "embed"

//go:embed base.eds

var f embed.FS
var rawDefaultOd []byte

// Return embeded default object dictionary
func Default() *ObjectDictionary {
	defaultOd, err := ParseV2(rawDefaultOd, 0)
	if err != nil {
		panic(err)
	}
	return defaultOd
}
