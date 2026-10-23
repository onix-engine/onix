// Package brotli implements the Brotli reader and writer needed by external
// formats: WOFF2 decoding and the WeChat WASM artifact. Resource .dat packages
// intentionally do not use this codec.
//
// The implementation is derived from github.com/andybalholm/brotli v1.2.1.
// HTTP helpers and the experimental matchfinder encoder are intentionally
// excluded so runtime builds do not acquire an unrelated net/http dependency.
package brotli
