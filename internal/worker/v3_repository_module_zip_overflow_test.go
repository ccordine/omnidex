package worker

import (
	"encoding/binary"
	"io"
	"math"
	"strings"
	"testing"
)

func TestRepositoryZIP64EndRecordRejectsOffsetAdditionOverflow(t *testing.T) {
	t.Parallel()
	locatorOffset := int64(math.MaxInt64 - 100)
	zip64Offset := int64(math.MaxInt64 - 200)
	reader := repositorySparseZIP64Reader{
		locatorOffset: locatorOffset,
		zip64Offset:   zip64Offset,
	}
	binary.LittleEndian.PutUint32(reader.locator[0:4], repositoryZip64LocatorSignature)
	binary.LittleEndian.PutUint64(reader.locator[8:16], uint64(zip64Offset))
	binary.LittleEndian.PutUint32(reader.locator[16:20], 1)
	binary.LittleEndian.PutUint32(reader.record[0:4], repositoryZip64EndSignature)
	binary.LittleEndian.PutUint64(reader.record[4:12], uint64(math.MaxInt64+1_000))

	_, err := readRepositoryModuleZip64End(
		reader, uint64(locatorOffset+repositoryZip64LocatorBytes),
		0, 0, math.MaxUint16, math.MaxUint16, math.MaxUint32, math.MaxUint32,
	)
	if err == nil || !strings.Contains(err.Error(), "end-record size overflows") {
		t.Fatalf("ZIP64 addition overflow error=%v", err)
	}
}

type repositorySparseZIP64Reader struct {
	locatorOffset int64
	zip64Offset   int64
	locator       [repositoryZip64LocatorBytes]byte
	record        [repositoryZip64EndBytes]byte
}

func (reader repositorySparseZIP64Reader) ReadAt(buffer []byte, offset int64) (int, error) {
	var source []byte
	switch offset {
	case reader.locatorOffset:
		source = reader.locator[:]
	case reader.zip64Offset:
		source = reader.record[:]
	default:
		return 0, io.EOF
	}
	if len(buffer) != len(source) {
		return 0, io.ErrShortBuffer
	}
	return copy(buffer, source), nil
}
