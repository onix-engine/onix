package project

import (
	"errors"
	"fmt"

	"github.com/diiyw/onix/render"
	"github.com/diiyw/onix/resource"
	onixtext "github.com/diiyw/onix/text"
)

type fontResource struct {
	font   *onixtext.Font
	source *resource.Blob
}

func (value fontResource) dispose() error {
	var fontErr, sourceErr error
	if value.font != nil {
		fontErr = value.font.Dispose()
	}
	if value.source != nil {
		sourceErr = value.source.Close()
	}
	return errors.Join(fontErr, sourceErr)
}

type shaderResource struct {
	descriptor render.ShaderDescriptor
	sources    []*resource.Blob
}

func (value shaderResource) dispose() error {
	errList := make([]error, 0, len(value.sources))
	for _, source := range value.sources {
		if source == nil {
			continue
		}
		if err := source.Close(); err != nil {
			errList = append(errList, err)
		}
	}
	return errors.Join(errList...)
}

// ResourceStatistics reports aggregate behavior of the typed runtime caches.
type ResourceStatistics struct {
	CacheHits         uint64
	CacheMisses       uint64
	ResidentResources uint64
}

type resourceStatisticsProvider interface {
	statistics() cacheStatistics
}

type resourceDisposer interface {
	disposeResources() error
}

func addResourceStatistics(target *ResourceStatistics, provider resourceStatisticsProvider) {
	if target == nil || provider == nil {
		return
	}
	statistics := provider.statistics()
	target.CacheHits += statistics.Hits
	target.CacheMisses += statistics.Misses
	target.ResidentResources += statistics.Resident
}

func disposeResource(kind string, disposer resourceDisposer, errList *[]error) {
	if disposer == nil {
		return
	}
	if err := disposer.disposeResources(); err != nil {
		*errList = append(*errList, fmt.Errorf("dispose %s resources: %w", kind, err))
	}
}
