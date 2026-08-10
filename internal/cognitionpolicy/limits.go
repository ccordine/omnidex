package cognitionpolicy

const (
	EnvelopeSchemaV2         = "omnidex.cognition-policy-envelope.v2"
	RendererVersionV2        = "omnidex.cognition-policy-renderer.v2"
	MaxProjectedContextBytes = 128 * 1024
	MaxEnvelopeBytes         = 512 * 1024
	MaxResponseBytes         = 64 * 1024
	MaxModelNameBytes        = 128
	MaxBrainIdentityBytes    = 256
	MaxNativeContextLimit    = 10_000_000
	MaxContextCeilingBytes   = 64 * 1024 * 1024
)
