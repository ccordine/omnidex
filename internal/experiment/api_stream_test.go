package experiment

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestDockerExecFramesPreserveDistinctBinaryStreams(t *testing.T) {
	frame := func(kind byte, value []byte) []byte {
		data := make([]byte, 8+len(value))
		data[0] = kind
		binary.BigEndian.PutUint32(data[4:8], uint32(len(value)))
		copy(data[8:], value)
		return data
	}
	input := append(frame(1, []byte{0, 255}), frame(2, []byte("diagnostic"))...)
	var stdout, stderr bytes.Buffer
	if err := readDockerExecStreams(bytes.NewReader(input), &stdout, &stderr); err != nil || !bytes.Equal(stdout.Bytes(), []byte{0, 255}) || stderr.String() != "diagnostic" {
		t.Fatalf("streams = %v %q, %v", stdout.Bytes(), stderr.String(), err)
	}
	for name, input := range map[string][]byte{
		"unknown stream":        frame(3, []byte("error")),
		"truncated header":      {1, 0, 0},
		"truncated payload":     frame(1, []byte("ab"))[:9],
		"invalid reserved byte": {1, 1, 0, 0, 0, 0, 0, 0},
	} {
		t.Run(name, func(t *testing.T) {
			if err := readDockerExecStreams(bytes.NewReader(input), &stdout, &stderr); err == nil {
				t.Fatal("invalid Docker stream accepted")
			}
		})
	}
}
