package internal

import "unicode/utf8"

const maxName = 200

func checks(name, email string, ids []string) bool {
	_ = len(name) > maxName // want `len\(\) of a string counts bytes`
	_ = 200 < len(name)     // want `len\(\) of a string counts bytes`
	_ = len(name) == 0      // empty is empty in bytes and in characters
	_ = utf8.RuneCountInString(name) > 200
	_ = len([]byte(email)) <= 254 // a limit in octets says so
	_ = len(ids) > 10             // a slice is not text
	_ = len([]rune(name)) > 40
	return len(name) > len(email) // two lengths, no limit
}
