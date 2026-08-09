package changeapply

import (
	"crypto/sha256"
	"encoding/hex"
)

func digest(raw []byte) string {
	value := sha256.Sum256(raw)
	return hex.EncodeToString(value[:])
}

func stageIdentity(snapshotID, contractID, patchHash string) string {
	hash := sha256.New()
	for _, value := range []string{snapshotID, contractID, patchHash} {
		_, _ = hash.Write([]byte(value))
		_, _ = hash.Write([]byte{0})
	}
	return "repository_change_stage_" + hex.EncodeToString(hash.Sum(nil))
}
