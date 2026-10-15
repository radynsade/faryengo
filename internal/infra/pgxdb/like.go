package pgxdb

import "strings"

//
// Pattern matching
//

var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

// LikeSubstring builds a LIKE or ILIKE pattern that matches value anywhere in
// the text. The pattern's own wildcards and backslashes in value are escaped
// with LIKE's default escape character, so they match only themselves.

func LikeSubstring(value string) string {
	return "%" + likeEscaper.Replace(value) + "%"
}
