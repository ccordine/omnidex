package workspace

import (
	"context"
	"fmt"
	"os"
)

const MaxDirectoryPageEntries = 256

type EntryKind string

const (
	EntryFile      EntryKind = "file"
	EntryDirectory EntryKind = "directory"
	EntryLink      EntryKind = "link"
	EntryOther     EntryKind = "other"
)

type Entry struct {
	Path string    `json:"path"`
	Kind EntryKind `json:"kind"`
	Mode uint32    `json:"mode"`
	Size int64     `json:"size"`
}

type File struct {
	Entry   Entry  `json:"entry"`
	Content []byte `json:"-"`
}

type DirectoryPage struct {
	Entries []Entry `json:"entries"`
	HasMore bool    `json:"has_more"`
}

type Reader interface {
	Stat(context.Context, string) (Entry, error)
	ReadFile(context.Context, string) (File, error)
	ReadDirectory(context.Context, string, string, int) (DirectoryPage, error)
}

type RootReader interface {
	Lstat(string) (os.FileInfo, error)
	Open(string) (*os.File, error)
}

func (entry Entry) Validate(exactPath string) error {
	if err := validateReadPath(exactPath); err != nil {
		return err
	}
	if entry.Path != exactPath || entry.Size < 0 || entry.Mode & ^uint32(0o777) != 0 {
		return fmt.Errorf("workspace entry differs from its requested path or valid filesystem values")
	}
	switch entry.Kind {
	case EntryFile, EntryDirectory, EntryLink, EntryOther:
		return nil
	default:
		return fmt.Errorf("workspace entry has an unregistered file kind")
	}
}

func entryFromInfo(relative string, info os.FileInfo) Entry {
	kind := EntryOther
	switch {
	case info.Mode().IsRegular():
		kind = EntryFile
	case info.IsDir():
		kind = EntryDirectory
	case info.Mode()&os.ModeSymlink != 0:
		kind = EntryLink
	}
	return Entry{Path: relative, Kind: kind, Mode: uint32(info.Mode().Perm()), Size: info.Size()}
}

func validateReadPath(relative string) error {
	if relative == "." {
		return nil
	}
	return validateRelativePath(relative)
}
