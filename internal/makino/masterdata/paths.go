package masterdata

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/google/uuid"
)

var encryptedFileName = regexp.MustCompile(`^[a-f0-9]{64}$`)

func StagingDirectory(root, id string) (string, error) {
	parsed, err := uuid.Parse(id)
	if err != nil || parsed.String() != id {
		return "", errors.New("invalid master import job ID")
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return "", err
	}
	for _, directory := range []string{root, filepath.Join(root, ".master-staging"), filepath.Join(root, ".master-staging", id)} {
		info, err := os.Lstat(directory)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return "", err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("master staging path is not a regular directory: %s", directory)
		}
	}
	return filepath.Join(root, ".master-staging", id), nil
}

func StagedPath(root, id, fileName string) (string, error) {
	if !encryptedFileName.MatchString(fileName) {
		return "", errors.New("invalid encrypted master filename")
	}
	directory, err := StagingDirectory(root, id)
	if err != nil {
		return "", err
	}
	return filepath.Join(directory, fileName), nil
}

func regularFile(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s is not a regular file", path)
	}
	return nil
}
