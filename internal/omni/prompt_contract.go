package omni

const MinimalOutputContract = "Be terse. Minimal words. No chat. No filler. No appeasement."

func withMinimalOutputContract(lines ...string) []string {
	out := make([]string, 0, len(lines)+1)
	out = append(out, MinimalOutputContract)
	out = append(out, lines...)
	return out
}
