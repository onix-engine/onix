package editor

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/diiyw/onix/resource"
)

const (
	resourceBuildPipelineVersion = 10
	resourceBuildReportVersion   = 2
	resourceBuildCacheFile       = "cache.json"
)

type resourceBuildReport struct {
	Version        int                          `json:"version"`
	Fingerprint    string                       `json:"fingerprint"`
	Platform       BuildPlatform                `json:"platform"`
	Mode           BuildMode                    `json:"mode"`
	CatalogVersion int                          `json:"catalogVersion"`
	PackageVersion int                          `json:"packageVersion"`
	Totals         resourceBuildReportTotals    `json:"totals"`
	Packages       []resourcePackageBuildReport `json:"packages"`
}

type resourceBuildReportTotals struct {
	Packages          int   `json:"packages"`
	Files             int   `json:"files"`
	RawBytes          int64 `json:"rawBytes"`
	PackedBytes       int64 `json:"packedBytes"`
	Stored            int   `json:"stored"`
	Deduplicated      int   `json:"deduplicated"`
	DeduplicatedBytes int64 `json:"deduplicatedBytes"`
}

type resourcePackageBuildReport struct {
	Name              string `json:"name"`
	File              string `json:"file"`
	Files             int    `json:"files"`
	RawBytes          int64  `json:"rawBytes"`
	PackedBytes       int64  `json:"packedBytes"`
	Stored            int    `json:"stored"`
	Deduplicated      int    `json:"deduplicated"`
	DeduplicatedBytes int64  `json:"deduplicatedBytes"`
}

type resourceBuildCacheManifest struct {
	Version     int                 `json:"version"`
	Fingerprint string              `json:"fingerprint"`
	Report      resourceBuildReport `json:"report"`
}

func newResourceBuildReport(fingerprint string, options resourcePackageOptions, capacity int) resourceBuildReport {
	return resourceBuildReport{
		Version: resourceBuildReportVersion, Fingerprint: fingerprint,
		Platform: options.Platform, Mode: options.Mode,
		CatalogVersion: resource.CatalogVersion, PackageVersion: resource.FormatVersion,
		Packages: make([]resourcePackageBuildReport, 0, capacity),
	}
}

func (report *resourceBuildReport) addPackage(name, file string, stats resource.PackStats) {
	if report == nil {
		return
	}
	report.Packages = append(report.Packages, resourcePackageBuildReport{
		Name: name, File: file, Files: stats.Files, RawBytes: stats.RawBytes,
		PackedBytes: stats.PackedBytes, Stored: stats.Stored,
		Deduplicated: stats.Deduplicated, DeduplicatedBytes: stats.DeduplicatedBytes,
	})
	report.Totals.Packages++
	report.Totals.Files += stats.Files
	report.Totals.RawBytes += stats.RawBytes
	report.Totals.PackedBytes += stats.PackedBytes
	report.Totals.Stored += stats.Stored
	report.Totals.Deduplicated += stats.Deduplicated
	report.Totals.DeduplicatedBytes += stats.DeduplicatedBytes
}

func (report *resourceBuildReport) sortPackages() {
	if report == nil {
		return
	}
	sort.Slice(report.Packages, func(i, j int) bool { return report.Packages[i].Name < report.Packages[j].Name })
}

func fingerprintResourceBuild(files map[string]resourceInput, config resourcePackageConfig, options resourcePackageOptions, digests *resourceDigestCache) (string, error) {
	hasher := sha256.New()
	writeFingerprintString(hasher, "onix-resource-build")
	writeFingerprintString(hasher, onixNagaCompilerVersion)
	writeFingerprintUint64(hasher, resourceBuildPipelineVersion)
	writeFingerprintUint64(hasher, resource.CatalogVersion)
	writeFingerprintUint64(hasher, resource.FormatVersion)
	writeFingerprintString(hasher, string(options.Platform))
	writeFingerprintString(hasher, string(options.Mode))
	configData, err := json.Marshal(config)
	if err != nil {
		return "", fmt.Errorf("encode resource package cache key: %w", err)
	}
	writeFingerprintBytes(hasher, configData)

	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	buffer := make([]byte, 64<<10)
	for index, name := range names {
		if options.Progress != nil {
			options.Progress(BuildProgress{Message: fmt.Sprintf("Checking resource %s (%d/%d)", name, index+1, len(names)), Value: .05 + .15*float32(index)/float32(max(1, len(names))), Indeterminate: true})
		}
		writeFingerprintString(hasher, name)
		digest, err := digests.digest(files[name], buffer)
		if err != nil {
			return "", fmt.Errorf("hash resource input %s: %w", name, err)
		}
		writeFingerprintBytes(hasher, digest[:])
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

// resourceDigestCache reuses content digests for sources whose size and
// modification time are unchanged, so repeat builds never re-read every asset.
type resourceDigestCache struct {
	path    string
	known   map[string]resourceDigestEntry
	entries map[string]resourceDigestEntry
	dirty   bool
}

type resourceDigestEntry struct {
	Size     int64  `json:"size"`
	Modified int64  `json:"modified"`
	Digest   string `json:"digest"`
}

func loadResourceDigestCache(projectDir string) *resourceDigestCache {
	path := filepath.Join(resourceBuildCacheRoot(projectDir), "digests.json")
	known, _ := readJSONFile[map[string]resourceDigestEntry](path)
	if known == nil {
		known = make(map[string]resourceDigestEntry)
	}
	return &resourceDigestCache{path: path, known: known, entries: make(map[string]resourceDigestEntry, len(known))}
}

// save drops entries for sources that vanished, so the file cannot grow forever.
func (c *resourceDigestCache) save() {
	if !c.dirty && len(c.entries) == len(c.known) {
		return
	}
	_ = writeJSONFile(c.path, c.entries)
}

func (c *resourceDigestCache) digest(input resourceInput, buffer []byte) ([sha256.Size]byte, error) {
	if input.memory {
		return sha256.Sum256(input.data), nil
	}
	stat, err := os.Stat(input.source)
	if err != nil {
		return [sha256.Size]byte{}, err
	}
	entry := resourceDigestEntry{Size: stat.Size(), Modified: stat.ModTime().UnixNano()}
	if known, ok := c.known[input.source]; ok && known.Size == entry.Size && known.Modified == entry.Modified {
		if raw, err := hex.DecodeString(known.Digest); err == nil && len(raw) == sha256.Size {
			entry.Digest = known.Digest
			c.entries[input.source] = entry
			var digest [sha256.Size]byte
			copy(digest[:], raw)
			return digest, nil
		}
	}
	file, err := os.Open(input.source)
	if err != nil {
		return [sha256.Size]byte{}, err
	}
	hasher := sha256.New()
	_, copyErr := io.CopyBuffer(hasher, file, buffer)
	closeErr := file.Close()
	if copyErr != nil || closeErr != nil {
		return [sha256.Size]byte{}, errors.Join(copyErr, closeErr)
	}
	var digest [sha256.Size]byte
	hasher.Sum(digest[:0])
	entry.Digest = hex.EncodeToString(digest[:])
	c.entries[input.source] = entry
	c.dirty = true
	return digest, nil
}

func writeFingerprintString(target hash.Hash, value string) {
	writeFingerprintBytes(target, []byte(value))
}

func writeFingerprintBytes(target hash.Hash, value []byte) {
	writeFingerprintUint64(target, uint64(len(value)))
	_, _ = target.Write(value)
}

func writeFingerprintUint64(target hash.Hash, value uint64) {
	var data [8]byte
	binary.LittleEndian.PutUint64(data[:], value)
	_, _ = target.Write(data[:])
}

func restoreCachedResourceBuild(projectDir, outputDir, fingerprint string, options resourcePackageOptions) (resource.Catalog, []byte, bool) {
	cacheDir := resourceBuildCacheDirectory(projectDir, options)
	manifest, err := readJSONFile[resourceBuildCacheManifest](filepath.Join(cacheDir, resourceBuildCacheFile))
	if err != nil || manifest.Version != resourceBuildPipelineVersion || manifest.Fingerprint != fingerprint ||
		manifest.Report.Version != resourceBuildReportVersion || manifest.Report.Fingerprint != fingerprint ||
		manifest.Report.CatalogVersion != resource.CatalogVersion || manifest.Report.PackageVersion != resource.FormatVersion {
		return resource.Catalog{}, nil, false
	}
	catalogData, err := os.ReadFile(filepath.Join(cacheDir, resource.CatalogFile))
	if err != nil {
		return resource.Catalog{}, nil, false
	}
	reportBuildActivity(options.Progress, "Validating cached resource packages...", .2)
	catalog, err := resource.ParseCatalog(catalogData)
	if err != nil || !cachedPackagesValid(cacheDir, catalog, options.Progress) {
		return resource.Catalog{}, nil, false
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return resource.Catalog{}, nil, false
	}
	for index, info := range catalog.Packages {
		if options.Progress != nil {
			options.Progress(BuildProgress{Message: fmt.Sprintf("Restoring module package %s (%d/%d, %d resources)", info.Name, index+1, len(catalog.Packages), len(info.Assets)), Value: .6 + .35*float32(index)/float32(max(1, len(catalog.Packages))), Indeterminate: true})
		}
		if err := linkOrCopyAtomic(filepath.Join(cacheDir, info.File), filepath.Join(outputDir, info.File)); err != nil {
			return resource.Catalog{}, nil, false
		}
	}
	if err := writeAtomic(filepath.Join(outputDir, resource.CatalogFile), catalogData); err != nil {
		return resource.Catalog{}, nil, false
	}
	if err := removeStalePackages(outputDir, catalog); err != nil {
		return resource.Catalog{}, nil, false
	}
	if options.Progress != nil {
		options.Progress(BuildProgress{Message: "Cached resource packages ready", Value: 1})
	}
	return catalog, catalogData, true
}

func cachedPackagesValid(cacheDir string, catalog resource.Catalog, progress func(BuildProgress)) bool {
	for index, info := range catalog.Packages {
		if progress != nil {
			progress(BuildProgress{Message: fmt.Sprintf("Checking cached module package %s (%d/%d)", info.Name, index+1, len(catalog.Packages)), Value: .2 + .4*float32(index)/float32(max(1, len(catalog.Packages))), Indeterminate: true})
		}
		file, err := os.Open(filepath.Join(cacheDir, info.File))
		if err != nil {
			return false
		}
		stat, statErr := file.Stat()
		hasher := sha256.New()
		_, hashErr := io.Copy(hasher, file)
		closeErr := file.Close()
		if statErr != nil || hashErr != nil || closeErr != nil || stat.Size() != info.Size {
			return false
		}
		var digest [sha256.Size]byte
		hasher.Sum(digest[:0])
		if digest != info.SHA256 {
			return false
		}
	}
	return true
}

func storeCachedResourceBuild(projectDir, outputDir string, catalog resource.Catalog, catalogData []byte, report resourceBuildReport) {
	cacheDir := resourceBuildCacheDirectory(projectDir, resourcePackageOptions{Platform: report.Platform, Mode: report.Mode})
	parent := filepath.Dir(cacheDir)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return
	}
	temporary, err := os.MkdirTemp(parent, filepath.Base(cacheDir)+".tmp-")
	if err != nil {
		return
	}
	cleanup := func() { _ = os.RemoveAll(temporary) }
	if err := os.WriteFile(filepath.Join(temporary, resource.CatalogFile), catalogData, 0o644); err != nil {
		cleanup()
		return
	}
	for _, info := range catalog.Packages {
		if err := linkOrCopyAtomic(filepath.Join(outputDir, info.File), filepath.Join(temporary, info.File)); err != nil {
			cleanup()
			return
		}
	}
	manifest := resourceBuildCacheManifest{
		Version: resourceBuildPipelineVersion, Fingerprint: report.Fingerprint, Report: report,
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		cleanup()
		return
	}
	data = append(data, '\n')
	if err := os.WriteFile(filepath.Join(temporary, resourceBuildCacheFile), data, 0o644); err != nil {
		cleanup()
		return
	}
	_ = os.RemoveAll(cacheDir)
	if err := os.Rename(temporary, cacheDir); err != nil {
		cleanup()
	}
}

func resourceBuildCacheDirectory(projectDir string, options resourcePackageOptions) string {
	key := string(options.Platform) + "-" + string(options.Mode)
	return filepath.Join(resourceBuildCacheRoot(projectDir), key)
}

func resourceBuildCacheRoot(projectDir string) string {
	return filepath.Join(projectDir, buildOutputDirName, ".onix-resource-cache")
}

func linkOrCopyAtomic(source, destination string) error {
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}
	temporary := destination + ".cache-tmp"
	_ = os.Remove(temporary)
	if err := os.Link(source, temporary); err != nil {
		if err := copyFile(source, temporary); err != nil {
			_ = os.Remove(temporary)
			return err
		}
	}
	if err := os.Remove(destination); err != nil && !os.IsNotExist(err) {
		_ = os.Remove(temporary)
		return err
	}
	if err := os.Rename(temporary, destination); err != nil {
		_ = os.Remove(temporary)
		return err
	}
	return os.Chmod(destination, 0o644)
}

func copyFile(source, destination string) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		_ = input.Close()
		return err
	}
	_, copyErr := io.Copy(output, input)
	closeOutputErr := output.Close()
	closeInputErr := input.Close()
	return errors.Join(copyErr, closeOutputErr, closeInputErr)
}
