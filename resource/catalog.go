package resource

import (
	"bytes"
	"cmp"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"path"
	"strings"
)

const (
	CatalogFile    = "catalog.bin"
	CatalogVersion = 2
	BasePackage    = "base"
	ContentIDSize  = 16
)

var catalogMagic = [8]byte{'O', 'N', 'I', 'X', 'C', 'A', 'T', '2'}

// ContentID identifies identical encoded resource contents independently of
// their logical paths. It is the first 128 bits of SHA-256.
type ContentID [ContentIDSize]byte

func contentID(data []byte) ContentID {
	digest := sha256.Sum256(data)
	return ContentID(digest[:ContentIDSize])
}

type Catalog struct {
	Version  int
	Packages []PackageInfo
}

type PackageInfo struct {
	Name         string
	File         string
	SHA256       [sha256.Size]byte
	Size         int64
	Required     bool
	Dependencies []string
	Assets       []AssetInfo
}

// AssetInfo is the catalog-owned metadata for one logical resource. Assets in
// each package are strictly sorted by Path and line up with the package index.
type AssetInfo struct {
	Path      string
	Size      int64
	Checksum  uint32
	ContentID ContentID
}

// MarshalCatalog encodes a validated catalog into Onix's compact binary format.
func MarshalCatalog(catalog Catalog) ([]byte, error) {
	if err := catalog.Validate(); err != nil {
		return nil, err
	}
	encoder := catalogEncoder{data: append([]byte(nil), catalogMagic[:]...)}
	encoder.uvarint(uint64(catalog.Version))
	encoder.uvarint(uint64(len(catalog.Packages)))
	for _, info := range catalog.Packages {
		encoder.string(info.Name)
		encoder.string(info.File)
		encoder.data = append(encoder.data, info.SHA256[:]...)
		encoder.uvarint(uint64(info.Size))
		encoder.bool(info.Required)
		encoder.strings(info.Dependencies)
		encoder.uvarint(uint64(len(info.Assets)))
		for _, asset := range info.Assets {
			encoder.string(asset.Path)
			encoder.uvarint(uint64(asset.Size))
			encoder.data = binary.LittleEndian.AppendUint32(encoder.data, asset.Checksum)
			encoder.data = append(encoder.data, asset.ContentID[:]...)
		}
	}
	return encoder.data, nil
}

// ParseCatalog decodes an ONIXCAT2 resource catalog.
func ParseCatalog(data []byte) (Catalog, error) {
	if len(data) < len(catalogMagic) || !bytes.Equal(data[:len(catalogMagic)], catalogMagic[:]) {
		return Catalog{}, errors.New("resource: invalid catalog format marker")
	}
	decoder := catalogDecoder{data: data[len(catalogMagic):]}
	version, err := decoder.integer()
	if err != nil {
		return Catalog{}, fmt.Errorf("resource: decode catalog version: %w", err)
	}
	count, err := decoder.length()
	if err != nil {
		return Catalog{}, fmt.Errorf("resource: decode catalog packages: %w", err)
	}
	catalog := Catalog{Version: version, Packages: make([]PackageInfo, count)}
	for index := range catalog.Packages {
		info := &catalog.Packages[index]
		if info.Name, err = decoder.string(); err != nil {
			return Catalog{}, fmt.Errorf("resource: decode catalog package %d name: %w", index, err)
		}
		if info.File, err = decoder.string(); err != nil {
			return Catalog{}, fmt.Errorf("resource: decode catalog package %q file: %w", info.Name, err)
		}
		digest, takeErr := decoder.take(sha256.Size)
		if takeErr != nil {
			return Catalog{}, fmt.Errorf("resource: decode catalog package %q SHA-256: %w", info.Name, takeErr)
		}
		copy(info.SHA256[:], digest)
		size, sizeErr := decoder.uvarint()
		if sizeErr != nil || size > uint64(maxInt64) {
			if sizeErr == nil {
				sizeErr = errors.New("size overflows int64")
			}
			return Catalog{}, fmt.Errorf("resource: decode catalog package %q size: %w", info.Name, sizeErr)
		}
		info.Size = int64(size)
		if info.Required, err = decoder.bool(); err != nil {
			return Catalog{}, fmt.Errorf("resource: decode catalog package %q required flag: %w", info.Name, err)
		}
		if info.Dependencies, err = decoder.strings(); err != nil {
			return Catalog{}, fmt.Errorf("resource: decode catalog package %q dependencies: %w", info.Name, err)
		}
		assetCount, countErr := decoder.length()
		if countErr != nil {
			return Catalog{}, fmt.Errorf("resource: decode catalog package %q assets: %w", info.Name, countErr)
		}
		info.Assets = make([]AssetInfo, assetCount)
		for assetIndex := range info.Assets {
			asset := &info.Assets[assetIndex]
			if asset.Path, err = decoder.string(); err != nil {
				return Catalog{}, fmt.Errorf("resource: decode catalog package %q asset %d path: %w", info.Name, assetIndex, err)
			}
			assetSize, assetErr := decoder.uvarint()
			if assetErr != nil || assetSize > uint64(maxInt64) {
				if assetErr == nil {
					assetErr = errors.New("size overflows int64")
				}
				return Catalog{}, fmt.Errorf("resource: decode catalog asset %q size: %w", asset.Path, assetErr)
			}
			asset.Size = int64(assetSize)
			checksum, checksumErr := decoder.take(4)
			if checksumErr != nil {
				return Catalog{}, fmt.Errorf("resource: decode catalog asset %q checksum: %w", asset.Path, checksumErr)
			}
			asset.Checksum = binary.LittleEndian.Uint32(checksum)
			id, idErr := decoder.take(ContentIDSize)
			if idErr != nil {
				return Catalog{}, fmt.Errorf("resource: decode catalog asset %q content ID: %w", asset.Path, idErr)
			}
			copy(asset.ContentID[:], id)
		}
	}
	if len(decoder.data) != 0 {
		return Catalog{}, fmt.Errorf("resource: catalog contains %d trailing bytes", len(decoder.data))
	}
	if err := catalog.Validate(); err != nil {
		return Catalog{}, err
	}
	return catalog, nil
}

const maxInt64 = int64(^uint64(0) >> 1)

type catalogEncoder struct{ data []byte }

func (e *catalogEncoder) uvarint(value uint64) {
	var buffer [binary.MaxVarintLen64]byte
	length := binary.PutUvarint(buffer[:], value)
	e.data = append(e.data, buffer[:length]...)
}

func (e *catalogEncoder) string(value string) {
	e.uvarint(uint64(len(value)))
	e.data = append(e.data, value...)
}

func (e *catalogEncoder) bool(value bool) {
	if value {
		e.data = append(e.data, 1)
		return
	}
	e.data = append(e.data, 0)
}

func (e *catalogEncoder) strings(values []string) {
	e.uvarint(uint64(len(values)))
	for _, value := range values {
		e.string(value)
	}
}

type catalogDecoder struct{ data []byte }

func (d *catalogDecoder) take(size int) ([]byte, error) {
	if size < 0 || size > len(d.data) {
		return nil, errors.New("truncated data")
	}
	value := d.data[:size]
	d.data = d.data[size:]
	return value, nil
}

func (d *catalogDecoder) uvarint() (uint64, error) {
	value, length := binary.Uvarint(d.data)
	if length == 0 {
		return 0, errors.New("truncated varint")
	}
	if length < 0 {
		return 0, errors.New("varint overflow")
	}
	d.data = d.data[length:]
	return value, nil
}

func (d *catalogDecoder) integer() (int, error) {
	value, err := d.uvarint()
	if err != nil {
		return 0, err
	}
	if value > uint64(^uint(0)>>1) {
		return 0, errors.New("integer overflow")
	}
	return int(value), nil
}

func (d *catalogDecoder) length() (int, error) {
	length, err := d.integer()
	if err != nil {
		return 0, err
	}
	if length > len(d.data) {
		return 0, errors.New("collection length exceeds remaining data")
	}
	return length, nil
}

func (d *catalogDecoder) string() (string, error) {
	length, err := d.length()
	if err != nil {
		return "", err
	}
	value, err := d.take(length)
	return string(value), err
}

func (d *catalogDecoder) bool() (bool, error) {
	value, err := d.take(1)
	if err != nil {
		return false, err
	}
	if value[0] > 1 {
		return false, fmt.Errorf("invalid bool %d", value[0])
	}
	return value[0] == 1, nil
}

func (d *catalogDecoder) strings() ([]string, error) {
	length, err := d.length()
	if err != nil {
		return nil, err
	}
	if length == 0 {
		return nil, nil
	}
	values := make([]string, length)
	for index := range values {
		if values[index], err = d.string(); err != nil {
			return nil, fmt.Errorf("item %d: %w", index, err)
		}
	}
	return values, nil
}

func (c Catalog) Validate() error {
	if c.Version != CatalogVersion {
		return fmt.Errorf("resource: unsupported catalog version %d", c.Version)
	}
	packages := make(map[string]PackageInfo, len(c.Packages))
	owners := make(map[string]string)
	previous := ""
	for _, info := range c.Packages {
		if !ValidPackageName(info.Name) {
			return fmt.Errorf("resource: invalid package name %q", info.Name)
		}
		if previous != "" && info.Name <= previous {
			return errors.New("resource: catalog packages are not strictly sorted")
		}
		previous = info.Name
		if info.File != info.Name+Extension {
			return fmt.Errorf("resource: package %q file must be %q", info.Name, info.Name+Extension)
		}
		if info.Size < headerSize {
			return fmt.Errorf("resource: package %q has invalid size %d", info.Name, info.Size)
		}
		if info.SHA256 == ([sha256.Size]byte{}) {
			return fmt.Errorf("resource: package %q has an empty SHA-256", info.Name)
		}
		if !strictlySorted(info.Dependencies) {
			return fmt.Errorf("resource: package %q dependencies must be sorted and unique", info.Name)
		}
		assetPrevious := ""
		for _, asset := range info.Assets {
			if err := validateResourcePath(asset.Path); err != nil {
				return err
			}
			if assetPrevious != "" && asset.Path <= assetPrevious {
				return fmt.Errorf("resource: package %q assets must be sorted and unique", info.Name)
			}
			assetPrevious = asset.Path
			if asset.Size < 0 {
				return fmt.Errorf("resource: asset %q has invalid size %d", asset.Path, asset.Size)
			}
			if asset.ContentID == (ContentID{}) {
				return fmt.Errorf("resource: asset %q has an empty content ID", asset.Path)
			}
			if owner, exists := owners[asset.Path]; exists {
				return fmt.Errorf("resource: file %q belongs to both %q and %q", asset.Path, owner, info.Name)
			}
			owners[asset.Path] = info.Name
		}
		packages[info.Name] = info
	}
	base, ok := packages[BasePackage]
	if !ok || !base.Required {
		return fmt.Errorf("resource: catalog requires a required %q package", BasePackage)
	}
	for name := range owners {
		for parent := path.Dir(name); parent != "."; parent = path.Dir(parent) {
			if _, exists := owners[parent]; exists {
				return fmt.Errorf("resource: catalog path %q is both a file and directory", parent)
			}
		}
	}
	for _, info := range c.Packages {
		for _, dependency := range info.Dependencies {
			if dependency == info.Name {
				return fmt.Errorf("resource: package %q depends on itself", info.Name)
			}
			if _, ok := packages[dependency]; !ok {
				return fmt.Errorf("resource: package %q dependency %q does not exist", info.Name, dependency)
			}
		}
	}
	state := make(map[string]uint8, len(packages))
	var visit func(string) error
	visit = func(name string) error {
		switch state[name] {
		case 1:
			return fmt.Errorf("resource: package dependency cycle includes %q", name)
		case 2:
			return nil
		}
		state[name] = 1
		for _, dependency := range packages[name].Dependencies {
			if err := visit(dependency); err != nil {
				return err
			}
		}
		state[name] = 2
		return nil
	}
	for name := range packages {
		if err := visit(name); err != nil {
			return err
		}
	}
	return nil
}

func ValidPackageName(name string) bool {
	if name == "" || name == "." || strings.HasSuffix(name, Extension) {
		return false
	}
	for _, char := range name {
		if char >= 'a' && char <= 'z' || char >= '0' && char <= '9' || char == '-' || char == '_' {
			continue
		}
		return false
	}
	return true
}

func strictlySorted[T cmp.Ordered](values []T) bool {
	for index := 1; index < len(values); index++ {
		if values[index] <= values[index-1] {
			return false
		}
	}
	return true
}
