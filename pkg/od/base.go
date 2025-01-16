package od

import _ "embed"

//go:embed base.eds
var rawDefaultOd []byte

// Return embeded default object dictionary
func Default() *ObjectDictionary {
	defaultOd, err := ParseV2(rawDefaultOd, 0)
	if err != nil {
		panic(err)
	}
	return defaultOd
}
