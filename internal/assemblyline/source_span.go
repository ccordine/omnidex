package assemblyline

type SourceSpan struct {
	StartLine int
	EndLine   int
}

func (s SourceSpan) Contains(line int) bool {
	return line >= s.StartLine && line <= s.EndLine
}
