package render

import (
	"errors"
	"fmt"
)

type driverFactory[S any] func(S) (Driver, error)

func newDriverWithFallback[S any](surface S, primaryName string, primary driverFactory[S], fallbackName string, fallback driverFactory[S]) (Driver, error) {
	driver, primaryErr := primary(surface)
	if primaryErr == nil {
		return driver, nil
	}
	driver, fallbackErr := fallback(surface)
	if fallbackErr == nil {
		return driver, nil
	}
	return nil, errors.Join(
		fmt.Errorf("render: %s driver: %w", primaryName, primaryErr),
		fmt.Errorf("render: %s fallback driver: %w", fallbackName, fallbackErr),
	)
}
