//go:build android || ios || (!linux && !darwin && !windows)

package resource

import "os"

func openPackageFile(name string) (PackageSource, error) {
	file, err := os.Open(name)
	if err != nil {
		return PackageSource{}, err
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return PackageSource{}, err
	}
	return PackageSource{ReaderAt: file, Size: info.Size(), Closer: file}, nil
}
