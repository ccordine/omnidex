package cognitiongauntlet

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
)

func hashExactFile(path string, maximumBytes int64) (string, error) {
	if path == "" || maximumBytes < 1 {
		return "", fmt.Errorf("artifact hash authority is invalid")
	}
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > maximumBytes {
		return "", fmt.Errorf("artifact is not one bounded regular file")
	}
	hash := sha256.New()
	written, err := io.Copy(hash, io.LimitReader(file, maximumBytes+1))
	if err != nil {
		return "", err
	}
	if written != info.Size() {
		return "", fmt.Errorf("artifact changed while hashing")
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func digestExactBytes(raw []byte) string {
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}
