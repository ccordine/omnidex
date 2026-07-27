package main

import (
	"path/filepath"
	"strings"
	"unicode"
)

func isVideoFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	_, ok := videoExtensions[ext]
	return ok
}

func normalizeForMatch(value string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(value)) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			continue
		}
		b.WriteByte(' ')
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

func naturalLess(a, b string) bool {
	return naturalCompare(filepath.ToSlash(strings.ToLower(a)), filepath.ToSlash(strings.ToLower(b))) < 0
}

func naturalCompare(a, b string) int {
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		ra := a[i]
		rb := b[j]
		if isDigitByte(ra) && isDigitByte(rb) {
			ai := i
			for i < len(a) && isDigitByte(a[i]) {
				i++
			}
			bj := j
			for j < len(b) && isDigitByte(b[j]) {
				j++
			}

			anum := strings.TrimLeft(a[ai:i], "0")
			bnum := strings.TrimLeft(b[bj:j], "0")
			if anum == "" {
				anum = "0"
			}
			if bnum == "" {
				bnum = "0"
			}
			if len(anum) != len(bnum) {
				if len(anum) < len(bnum) {
					return -1
				}
				return 1
			}
			if anum != bnum {
				if anum < bnum {
					return -1
				}
				return 1
			}
			if (i - ai) != (j - bj) {
				if (i - ai) < (j - bj) {
					return -1
				}
				return 1
			}
			continue
		}

		if ra != rb {
			if ra < rb {
				return -1
			}
			return 1
		}
		i++
		j++
	}
	if len(a) == len(b) {
		return 0
	}
	if len(a) < len(b) {
		return -1
	}
	return 1
}

func isDigitByte(b byte) bool {
	return b >= '0' && b <= '9'
}

func safeValue(value, fallback string) string {
	clean := strings.TrimSpace(value)
	if clean == "" {
		return fallback
	}
	return clean
}
