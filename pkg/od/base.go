package od

import _ "embed"

//go:embed base.eds
var rawDefaultOd []byte

// Return embeded default object dictionary
func Default() *ObjectDictionary {
	defaultOd, err := Parse(rawDefaultOd, 0)
	if err != nil {
		panic(err)
	}
	return defaultOd
}
