package worker

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"os"
)

const (
	maxRepositoryGoModuleCentralDirectoryBytes uint64 = 64 << 20
	repositoryZipEndSignature                         = 0x06054b50
	repositoryZip64EndSignature                       = 0x06064b50
	repositoryZip64LocatorSignature                   = 0x07064b50
	repositoryZipCentralDirectorySignature            = 0x02014b50
	repositoryZipEndBytes                             = 22
	repositoryZip64EndBytes                           = 56
	repositoryZip64LocatorBytes                       = 20
	repositoryZipCentralHeaderBytes                   = 46
	repositoryZipMaxCommentBytes                      = 1<<16 - 1
)

type repositoryModuleZipDirectory struct {
	Entries int
	Bytes   int64
}

type repositoryModuleZipEnd struct {
	entries         uint64
	directoryBytes  uint64
	directoryOffset uint64
	directoryEnd    uint64
}

func preflightRepositoryGoModuleZip(
	path string,
	maxEntries int,
) (repositoryModuleZipDirectory, error) {
	if maxEntries < 1 || maxEntries > maxRepositoryGoModuleViewEntries {
		return repositoryModuleZipDirectory{}, fmt.Errorf(
			"repository module ZIP entry allowance must be between 1 and %d",
			maxRepositoryGoModuleViewEntries,
		)
	}
	before, err := os.Lstat(path)
	if err != nil || !before.Mode().IsRegular() || before.Mode()&os.ModeSymlink != 0 ||
		before.Size() < repositoryZipEndBytes {
		return repositoryModuleZipDirectory{}, fmt.Errorf("repository module ZIP is absent, unsafe, or malformed")
	}
	handle, err := os.Open(path)
	if err != nil {
		return repositoryModuleZipDirectory{}, fmt.Errorf("open repository module ZIP preflight: %w", err)
	}
	defer handle.Close()
	opened, err := handle.Stat()
	if err != nil || !opened.Mode().IsRegular() || !os.SameFile(before, opened) {
		return repositoryModuleZipDirectory{}, fmt.Errorf("repository module ZIP changed while it was opened")
	}
	end, err := readRepositoryModuleZipEnd(handle, opened.Size())
	if err != nil {
		return repositoryModuleZipDirectory{}, err
	}
	if end.entries > uint64(maxEntries) {
		return repositoryModuleZipDirectory{}, fmt.Errorf(
			"repository module ZIP exceeds remaining exact %d-entry limit", maxEntries,
		)
	}
	if end.directoryBytes > maxRepositoryGoModuleCentralDirectoryBytes {
		return repositoryModuleZipDirectory{}, fmt.Errorf(
			"repository module ZIP exceeds exact %d-central-directory byte limit",
			maxRepositoryGoModuleCentralDirectoryBytes,
		)
	}
	entries, err := scanRepositoryModuleZipDirectory(handle, end, maxEntries)
	if err != nil {
		return repositoryModuleZipDirectory{}, err
	}
	after, err := os.Lstat(path)
	if err != nil || !os.SameFile(before, after) || before.Size() != after.Size() ||
		before.Mode() != after.Mode() || !before.ModTime().Equal(after.ModTime()) {
		return repositoryModuleZipDirectory{}, fmt.Errorf("repository module ZIP changed during bounded preflight")
	}
	return repositoryModuleZipDirectory{Entries: entries, Bytes: int64(end.directoryBytes)}, nil
}

func readRepositoryModuleZipEnd(reader io.ReaderAt, size int64) (repositoryModuleZipEnd, error) {
	if size < repositoryZipEndBytes {
		return repositoryModuleZipEnd{}, fmt.Errorf("repository module ZIP has no exact end record")
	}
	tailBytes := int64(repositoryZipEndBytes + repositoryZipMaxCommentBytes)
	if size < tailBytes {
		tailBytes = size
	}
	tail := make([]byte, int(tailBytes))
	if _, err := reader.ReadAt(tail, size-tailBytes); err != nil {
		return repositoryModuleZipEnd{}, fmt.Errorf("read bounded repository module ZIP tail: %w", err)
	}
	relative := -1
	for index := len(tail) - repositoryZipEndBytes; index >= 0; index-- {
		if binary.LittleEndian.Uint32(tail[index:index+4]) != repositoryZipEndSignature {
			continue
		}
		commentBytes := int(binary.LittleEndian.Uint16(tail[index+20 : index+22]))
		if index+repositoryZipEndBytes+commentBytes == len(tail) {
			relative = index
			break
		}
	}
	if relative < 0 {
		return repositoryModuleZipEnd{}, fmt.Errorf("repository module ZIP has no exact end record")
	}
	eocdOffset := uint64(size-tailBytes) + uint64(relative)
	record := tail[relative : relative+repositoryZipEndBytes]
	disk := binary.LittleEndian.Uint16(record[4:6])
	directoryDisk := binary.LittleEndian.Uint16(record[6:8])
	entriesOnDisk := binary.LittleEndian.Uint16(record[8:10])
	entries := binary.LittleEndian.Uint16(record[10:12])
	directoryBytes := binary.LittleEndian.Uint32(record[12:16])
	directoryOffset := binary.LittleEndian.Uint32(record[16:20])
	zip64 := entriesOnDisk == math.MaxUint16 || entries == math.MaxUint16 ||
		directoryBytes == math.MaxUint32 || directoryOffset == math.MaxUint32 ||
		disk == math.MaxUint16 || directoryDisk == math.MaxUint16
	if !zip64 {
		if disk != 0 || directoryDisk != 0 || entriesOnDisk != entries {
			return repositoryModuleZipEnd{}, fmt.Errorf("repository module ZIP uses unsupported multi-disk authority")
		}
		return validateRepositoryModuleZipEnd(repositoryModuleZipEnd{
			entries: uint64(entries), directoryBytes: uint64(directoryBytes),
			directoryOffset: uint64(directoryOffset), directoryEnd: eocdOffset,
		})
	}
	return readRepositoryModuleZip64End(
		reader, eocdOffset, disk, directoryDisk, entriesOnDisk, entries,
		directoryBytes, directoryOffset,
	)
}

func readRepositoryModuleZip64End(
	reader io.ReaderAt,
	eocdOffset uint64,
	classicDisk, classicDirectoryDisk, classicEntriesOnDisk, classicEntries uint16,
	classicDirectoryBytes, classicDirectoryOffset uint32,
) (repositoryModuleZipEnd, error) {
	if eocdOffset < repositoryZip64LocatorBytes {
		return repositoryModuleZipEnd{}, fmt.Errorf("repository ZIP64 module has no exact locator")
	}
	locatorOffset := eocdOffset - repositoryZip64LocatorBytes
	var locator [repositoryZip64LocatorBytes]byte
	if _, err := reader.ReadAt(locator[:], int64(locatorOffset)); err != nil {
		return repositoryModuleZipEnd{}, fmt.Errorf("read repository ZIP64 locator: %w", err)
	}
	if binary.LittleEndian.Uint32(locator[0:4]) != repositoryZip64LocatorSignature ||
		binary.LittleEndian.Uint32(locator[4:8]) != 0 ||
		binary.LittleEndian.Uint32(locator[16:20]) != 1 {
		return repositoryModuleZipEnd{}, fmt.Errorf("repository ZIP64 module has malformed or multi-disk locator")
	}
	zip64Offset := binary.LittleEndian.Uint64(locator[8:16])
	if zip64Offset > math.MaxInt64 || zip64Offset >= locatorOffset {
		return repositoryModuleZipEnd{}, fmt.Errorf("repository ZIP64 module has invalid end-record offset")
	}
	var record [repositoryZip64EndBytes]byte
	if _, err := reader.ReadAt(record[:], int64(zip64Offset)); err != nil {
		return repositoryModuleZipEnd{}, fmt.Errorf("read repository ZIP64 end record: %w", err)
	}
	recordSize := binary.LittleEndian.Uint64(record[4:12])
	if binary.LittleEndian.Uint32(record[0:4]) != repositoryZip64EndSignature || recordSize < 44 {
		return repositoryModuleZipEnd{}, fmt.Errorf("repository ZIP64 module has malformed end record")
	}
	if recordSize > math.MaxUint64-12 || zip64Offset > math.MaxUint64-12-recordSize {
		return repositoryModuleZipEnd{}, fmt.Errorf("repository ZIP64 module end-record size overflows")
	}
	if zip64Offset+12+recordSize != locatorOffset {
		return repositoryModuleZipEnd{}, fmt.Errorf("repository ZIP64 module has malformed end record")
	}
	if binary.LittleEndian.Uint32(record[16:20]) != 0 ||
		binary.LittleEndian.Uint32(record[20:24]) != 0 {
		return repositoryModuleZipEnd{}, fmt.Errorf("repository ZIP64 module uses unsupported multi-disk authority")
	}
	entriesOnDisk := binary.LittleEndian.Uint64(record[24:32])
	entries := binary.LittleEndian.Uint64(record[32:40])
	if entriesOnDisk != entries {
		return repositoryModuleZipEnd{}, fmt.Errorf("repository ZIP64 module uses unsupported multi-disk entries")
	}
	end := repositoryModuleZipEnd{
		entries: entries, directoryBytes: binary.LittleEndian.Uint64(record[40:48]),
		directoryOffset: binary.LittleEndian.Uint64(record[48:56]), directoryEnd: zip64Offset,
	}
	if err := requireRepositoryZip64ClassicAgreement(
		end, classicDisk, classicDirectoryDisk, classicEntriesOnDisk, classicEntries,
		classicDirectoryBytes, classicDirectoryOffset,
	); err != nil {
		return repositoryModuleZipEnd{}, err
	}
	return validateRepositoryModuleZipEnd(end)
}

func requireRepositoryZip64ClassicAgreement(
	end repositoryModuleZipEnd,
	disk, directoryDisk, entriesOnDisk, entries uint16,
	directoryBytes, directoryOffset uint32,
) error {
	if disk != 0 && disk != math.MaxUint16 || directoryDisk != 0 && directoryDisk != math.MaxUint16 {
		return fmt.Errorf("repository ZIP64 module has conflicting multi-disk authority")
	}
	if entriesOnDisk != math.MaxUint16 && uint64(entriesOnDisk) != end.entries ||
		entries != math.MaxUint16 && uint64(entries) != end.entries ||
		directoryBytes != math.MaxUint32 && uint64(directoryBytes) != end.directoryBytes ||
		directoryOffset != math.MaxUint32 && uint64(directoryOffset) != end.directoryOffset {
		return fmt.Errorf("repository ZIP64 module disagrees with its classic end record")
	}
	return nil
}

func validateRepositoryModuleZipEnd(
	end repositoryModuleZipEnd,
) (repositoryModuleZipEnd, error) {
	if end.directoryBytes > maxRepositoryGoModuleCentralDirectoryBytes {
		return repositoryModuleZipEnd{}, fmt.Errorf(
			"repository module ZIP exceeds exact %d-central-directory byte limit",
			maxRepositoryGoModuleCentralDirectoryBytes,
		)
	}
	if end.directoryOffset > math.MaxInt64 || end.directoryBytes > math.MaxInt64 ||
		end.directoryOffset > math.MaxUint64-end.directoryBytes ||
		end.directoryOffset+end.directoryBytes != end.directoryEnd {
		return repositoryModuleZipEnd{}, fmt.Errorf("repository module ZIP has malformed central-directory geometry")
	}
	return end, nil
}

func scanRepositoryModuleZipDirectory(
	reader io.ReaderAt,
	end repositoryModuleZipEnd,
	maxEntries int,
) (int, error) {
	offset := end.directoryOffset
	limit := end.directoryOffset + end.directoryBytes
	entries := 0
	for offset < limit {
		if limit-offset < repositoryZipCentralHeaderBytes || offset > math.MaxInt64 {
			return 0, fmt.Errorf("repository module ZIP central directory is truncated")
		}
		var header [repositoryZipCentralHeaderBytes]byte
		if _, err := reader.ReadAt(header[:], int64(offset)); err != nil {
			return 0, fmt.Errorf("read bounded repository module ZIP central header: %w", err)
		}
		if binary.LittleEndian.Uint32(header[0:4]) != repositoryZipCentralDirectorySignature {
			return 0, fmt.Errorf("repository module ZIP central directory has an invalid entry signature")
		}
		if disk := binary.LittleEndian.Uint16(header[34:36]); disk != 0 {
			return 0, fmt.Errorf("repository module ZIP central entry uses unsupported multi-disk authority")
		}
		variableBytes := uint64(binary.LittleEndian.Uint16(header[28:30])) +
			uint64(binary.LittleEndian.Uint16(header[30:32])) +
			uint64(binary.LittleEndian.Uint16(header[32:34]))
		recordBytes := uint64(repositoryZipCentralHeaderBytes) + variableBytes
		if recordBytes > limit-offset {
			return 0, fmt.Errorf("repository module ZIP central entry exceeds its bounded directory")
		}
		entries++
		if entries > maxEntries {
			return 0, fmt.Errorf(
				"repository module ZIP exceeds remaining exact %d-entry limit", maxEntries,
			)
		}
		offset += recordBytes
	}
	if uint64(entries) != end.entries {
		return 0, fmt.Errorf(
			"repository module ZIP central entry count=%d differs from declared=%d",
			entries, end.entries,
		)
	}
	return entries, nil
}
