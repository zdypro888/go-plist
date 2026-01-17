package plist

import "strconv"

// mustParseInt parses a string as an int64, panicking on error.
// Used by parsers which use panic/recover for error handling.
func mustParseInt(str string, base, bits int) int64 {
	i, err := strconv.ParseInt(str, base, bits)
	if err != nil {
		panic(err)
	}
	return i
}

// mustParseUint parses a string as a uint64, panicking on error.
// Used by parsers which use panic/recover for error handling.
func mustParseUint(str string, base, bits int) uint64 {
	i, err := strconv.ParseUint(str, base, bits)
	if err != nil {
		panic(err)
	}
	return i
}

// mustParseFloat parses a string as a float64, panicking on error.
// Used by parsers which use panic/recover for error handling.
func mustParseFloat(str string, bits int) float64 {
	f, err := strconv.ParseFloat(str, bits)
	if err != nil {
		panic(err)
	}
	return f
}
