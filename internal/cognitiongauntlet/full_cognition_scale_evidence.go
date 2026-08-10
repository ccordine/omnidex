package cognitiongauntlet

import "fmt"

func labyrinthRelevantSurfaceBytes(fixture MicrogauntletCase) (int64, error) {
	public := fixture.generated.PublicArtifact()
	oracle := fixture.generated.PrivateOracle()
	records := make(map[string]struct {
		sha     string
		content string
	}, len(public.World.Descriptor.Records))
	for _, record := range public.World.Descriptor.Records {
		records[string(record.ID)] = struct {
			sha     string
			content string
		}{record.ContentSHA256, record.Content}
	}
	var total int64
	for _, evidence := range oracle.RequiredEvidence {
		record, exists := records[evidence.ID]
		if !exists || record.sha != evidence.SHA256 {
			return 0, fmt.Errorf("scale relevant evidence is absent from the sealed public surface")
		}
		total += int64(len([]byte(record.content)))
	}
	if total <= 0 {
		return 0, fmt.Errorf("scale relevant surface has no bytes")
	}
	return total, nil
}
