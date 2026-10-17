package codingagent

import "strings"

// Sentence-sized source units let classification separate a lasting obligation
// from a session-only observation in the same paragraph. Every byte stays in
// its original order; semantic grouping still belongs to the existing scan.
func parserSentenceFragments(line string) []string {
	fragments := []string{}
	start, codeTicks := 0, 0
	for index := 0; index < len(line); index++ {
		if line[index] == '`' {
			end := index + 1
			for end < len(line) && line[end] == '`' {
				end++
			}
			count := end - index
			if codeTicks == 0 {
				codeTicks = count
			} else if codeTicks == count {
				codeTicks = 0
			}
			index = end - 1
			continue
		}
		if codeTicks != 0 || !strings.ContainsRune(".!?", rune(line[index])) {
			continue
		}
		if line[index] == '.' && parserListOrdinal(line[start:index]) {
			continue
		}
		end := index + 1
		for end < len(line) && (line[end] == ' ' || line[end] == '\t') {
			end++
		}
		if end == index+1 || end == len(line) || line[end] == '\n' || line[end] == '\r' {
			continue
		}
		fragments = append(fragments, line[start:end])
		start, index = end, end-1
	}
	return append(fragments, line[start:])
}

func parserListOrdinal(prefix string) bool {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return false
	}
	for _, char := range prefix {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}
