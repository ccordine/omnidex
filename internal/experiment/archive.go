package experiment

import (
	"archive/tar"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"
)

const containerUserID = 65532

func writeInputArchive(writer io.Writer, files []File) error {
	archive := tar.NewWriter(writer)
	if err := archive.WriteHeader(&tar.Header{
		Name: "workspace/", Typeflag: tar.TypeDir, Mode: 0o755,
		Uid: containerUserID, Gid: containerUserID,
	}); err != nil {
		return err
	}
	directories := make(map[string]struct{})
	for _, file := range files {
		for parent := path.Dir(file.Path); parent != "."; parent = path.Dir(parent) {
			directories[parent] = struct{}{}
		}
	}
	ordered := make([]string, 0, len(directories))
	for directory := range directories {
		ordered = append(ordered, directory)
	}
	sort.Strings(ordered)
	for _, directory := range ordered {
		if err := archive.WriteHeader(&tar.Header{
			Name: "workspace/" + directory + "/", Typeflag: tar.TypeDir, Mode: 0o755,
			Uid: containerUserID, Gid: containerUserID,
		}); err != nil {
			return err
		}
	}
	for _, file := range files {
		if err := archive.WriteHeader(&tar.Header{
			Name: "workspace/" + file.Path, Typeflag: tar.TypeReg, Mode: int64(file.Mode), Size: int64(len(file.Content)),
			Uid: containerUserID, Gid: containerUserID,
		}); err != nil {
			return err
		}
		if _, err := archive.Write(file.Content); err != nil {
			return err
		}
	}
	return archive.Close()
}

// Docker streams the workspace as tar data. Nothing is extracted onto the host.
// Only declared regular files and their exact directory ancestry are consumed.
func readCollectedArchive(reader io.Reader, paths []string) ([]File, error) {
	return readArtifactArchive(reader, paths, "")
}

func readArtifactArchive(reader io.Reader, paths []string, subtree string) ([]File, error) {
	limited := &io.LimitedReader{R: reader, N: MaxArchiveBytes + 1}
	archive := tar.NewReader(limited)
	wanted := make(map[string]struct{}, len(paths))
	parents := map[string]struct{}{"workspace": {}}
	if subtree != "" {
		for parent := "workspace/" + subtree; parent != "."; parent = path.Dir(parent) {
			parents[parent] = struct{}{}
		}
	}
	for _, name := range paths {
		wanted["workspace/"+name] = struct{}{}
		for parent := path.Dir("workspace/" + name); parent != "."; parent = path.Dir(parent) {
			parents[parent] = struct{}{}
		}
	}
	observed := make(map[string]byte)
	files := make(map[string]File, len(paths))
	bytes := int64(0)
	for count := 0; ; count++ {
		header, err := archive.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read experiment artifact archive: %w", err)
		}
		if count >= 100_000 {
			return nil, fmt.Errorf("experiment archive exceeds its entry bound")
		}
		name := strings.TrimSuffix(header.Name, "/")
		if err := validatePath(name); err != nil {
			return nil, err
		}
		if name != "workspace" && !strings.HasPrefix(name, "workspace/") {
			return nil, fmt.Errorf("experiment archive path %q is outside its workspace", name)
		}
		if _, duplicate := observed[name]; duplicate {
			return nil, fmt.Errorf("experiment archive repeats path %q", name)
		}
		observed[name] = header.Typeflag
		if subtree != "" && strings.HasPrefix(name, "workspace/"+subtree+"/") {
			if header.Typeflag == tar.TypeDir {
				parents[name] = struct{}{}
			} else {
				if len(wanted) >= MaxFiles {
					return nil, fmt.Errorf("experiment artifact tree exceeds its file count bound")
				}
				wanted[name] = struct{}{}
				for parent := path.Dir(name); parent != "."; parent = path.Dir(parent) {
					parents[parent] = struct{}{}
				}
			}
		}
		if _, parent := parents[name]; parent && header.Typeflag != tar.TypeDir {
			return nil, fmt.Errorf("experiment artifact parent %q is not an exact directory", name)
		}
		if _, collect := wanted[name]; !collect {
			continue
		}
		if header.Typeflag != tar.TypeReg || header.Size < 0 || header.Size > MaxFileBytes ||
			header.Mode <= 0 || header.Mode&^int64(0o777) != 0 {
			return nil, fmt.Errorf("experiment artifact %q is not a bounded regular file", name)
		}
		bytes += header.Size
		if bytes > MaxInputBytes {
			return nil, fmt.Errorf("collected experiment artifacts exceed their byte bound")
		}
		content, err := io.ReadAll(archive)
		if err != nil {
			return nil, fmt.Errorf("experiment artifact %q has incomplete content: %w", name, err)
		}
		if int64(len(content)) != header.Size {
			return nil, fmt.Errorf("experiment artifact %q has %d content bytes; expected %d", name, len(content), header.Size)
		}
		files[name] = File{Path: strings.TrimPrefix(name, "workspace/"), Content: content, Mode: uint32(header.Mode)}
	}
	if _, err := io.Copy(io.Discard, limited); err != nil {
		return nil, fmt.Errorf("finish experiment archive observation: %w", err)
	}
	if limited.N == 0 {
		return nil, fmt.Errorf("experiment archive exceeds its total byte bound")
	}
	for parent := range parents {
		if kind, exists := observed[parent]; !exists || kind != tar.TypeDir {
			return nil, fmt.Errorf("experiment artifact parent %q was not observed as an exact directory", parent)
		}
	}
	if subtree != "" {
		paths = make([]string, 0, len(files))
		for name := range files {
			paths = append(paths, strings.TrimPrefix(name, "workspace/"))
		}
		sort.Strings(paths)
	}
	result := make([]File, 0, len(paths))
	for _, name := range paths {
		file, exists := files["workspace/"+name]
		if !exists {
			return nil, fmt.Errorf("declared experiment artifact %q is missing", name)
		}
		result = append(result, file)
	}
	return result, nil
}
