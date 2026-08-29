package assemblyline

const (
	// PortableRendererV8 identifies the sole renderer used for new portable
	// station openings. Prompt changes require a new identity so an immutable
	// historical opening is never reinterpreted through different bytes.
	PortableRendererV8 = "omnidex.render-portable-job.v8"

	// HistoricalPortableRendererV7 identifies frozen prompt-only evidence that
	// may be replayed from its stored bytes. There is deliberately no V7 prompt
	// renderer: current code must not reconstruct historical model context.
	HistoricalPortableRendererV7 = "omnidex.render-portable-job.v7"

	// HistoricalPortableRendererV6 identifies frozen prompt-only evidence that
	// may be replayed from its stored bytes. There is deliberately no V6 prompt
	// renderer: current code must not reconstruct historical model context.
	HistoricalPortableRendererV6 = "omnidex.render-portable-job.v6"

	// HistoricalPortableRendererV5 identifies frozen prompt-only evidence that
	// may be replayed from its stored bytes. There is deliberately no V5 prompt
	// renderer: current code must not reconstruct historical model context.
	HistoricalPortableRendererV5 = "omnidex.render-portable-job.v5"
)

func IsReplayablePortableRenderer(renderer string) bool {
	return renderer == PortableRendererV8 ||
		renderer == HistoricalPortableRendererV7 ||
		renderer == HistoricalPortableRendererV6 ||
		renderer == HistoricalPortableRendererV5
}
