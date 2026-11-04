package style

import (
	"fmt"
	"runtime"
	"strings"
	"sync"
	"testing"
	"unsafe"

	"github.com/diiyw/onix/html/dom"
)

func TestInlineStyleCacheOwnsSource(t *testing.T) {
	const input = "display:block;color:red;transition:color .2s;animation:fade 1s"
	source := strings.Repeat("x", 1<<20) + input + strings.Repeat("y", 1<<20)
	var cache inlineDeclarationCache
	values, err := cache.parse(source[1<<20 : 1<<20+len(input)])
	if err != nil || len(values) != 4 {
		t.Fatalf("parse inline: %v", err)
	}
	check := func(value string) {
		t.Helper()
		address, start := uintptr(unsafe.Pointer(unsafe.StringData(value))), uintptr(unsafe.Pointer(unsafe.StringData(source)))
		if address >= start && address < start+uintptr(len(source)) {
			t.Fatalf("cached string %q still pins the HTML source", value)
		}
	}
	for key := range cache.entries {
		check(key)
	}
	for _, declaration := range values {
		check(declaration.property)
		check(declaration.value)
		if parsed := declaration.parsed; parsed != nil {
			check(parsed.property)
			check(parsed.value.Keyword)
			if motion := parsed.motion; motion != nil {
				for _, value := range motion.transition {
					check(value)
				}
				for _, value := range motion.animation {
					check(value)
				}
			}
		}
	}
	runtime.KeepAlive(source)
}

func TestInlineStyleCacheSurvivesParserReuse(t *testing.T) {
	var parser dom.Parser
	root, err := parser.Parse(`<div style="color:r&#101;d"></div>`)
	if err != nil {
		t.Fatal(err)
	}
	input, _ := root.Children[0].Attr("style")
	var cache inlineDeclarationCache
	values, err := cache.parse(input)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parser.Parse(`<div style="color:t&#97;n"></div>`); err != nil {
		t.Fatal(err)
	}
	if len(values) != 1 || values[0].value != "red" {
		t.Fatal("reusing the parser changed cached declarations")
	}
	again, err := cache.parse("color:red")
	if err != nil || &again[0] != &values[0] || len(cache.entries) != 1 {
		t.Fatal("reusing the parser corrupted the inline cache key")
	}
}

func TestInlineStyleCacheLimits(t *testing.T) {
	t.Run("bytes", func(t *testing.T) {
		var cache inlineDeclarationCache
		base := strings.Repeat("width:1px;", 512)
		first := base + "height:0px"
		values, err := cache.parse(first)
		if err != nil {
			t.Fatal(err)
		}
		for i := 1; i <= 128; i++ {
			if _, err := cache.parse(fmt.Sprintf("%sheight:%dpx", base, i)); err != nil {
				t.Fatal(err)
			}
			if cache.bytes > inlineStyleCacheBytesLimit {
				t.Fatalf("inline payload exceeded budget: %d", cache.bytes)
			}
		}
		if _, exists := cache.entries[first]; exists || len(cache.entries) >= 129 {
			t.Fatal("parsed declaration storage did not trigger eviction")
		}
		if values[0].value != "1px" || values[len(values)-1].value != "0px" {
			t.Fatal("eviction invalidated declarations held by a caller")
		}
	})
	t.Run("count", func(t *testing.T) {
		var cache inlineDeclarationCache
		for i := range inlineStyleCacheLimit + 1 {
			if _, err := cache.parse(fmt.Sprintf("--ignored:%d", i)); err != nil {
				t.Fatal(err)
			}
		}
		if len(cache.entries) != 1 || cache.bytes > inlineStyleCacheBytesLimit {
			t.Fatalf("entry limit not enforced: %d entries, %d bytes", len(cache.entries), cache.bytes)
		}
	})
	t.Run("oversized", func(t *testing.T) {
		var cache inlineDeclarationCache
		cache.parse("color:red")
		before := cache.bytes
		values, err := cache.parse("/*" + strings.Repeat("x", inlineStyleCacheSourceLimit) + "*/color:blue")
		if err != nil || len(values) != 1 || values[0].value != "blue" {
			t.Fatal("oversized inline style was not parsed")
		}
		if len(cache.entries) != 1 || cache.bytes != before {
			t.Fatal("oversized source displaced the working set")
		}
	})
}

func TestInlineStyleCacheConcurrentMissAndWarmHit(t *testing.T) {
	const input = "width:10px;color:red;transition:color .2s"
	var cache inlineDeclarationCache
	var wg sync.WaitGroup
	start := make(chan struct{})
	for range 32 {
		wg.Go(func() {
			<-start
			if values, err := cache.parse(input); err != nil || len(values) != 3 {
				t.Errorf("concurrent inline parsing failed: %v", err)
			}
		})
	}
	close(start)
	wg.Wait()
	_, bytes, err := parseInlineDeclarations(input)
	if err != nil || len(cache.entries) != 1 || cache.bytes != bytes {
		t.Fatalf("concurrent misses double-counted payload: entries=%d bytes=%d want=%d", len(cache.entries), cache.bytes, bytes)
	}
	if allocs := testing.AllocsPerRun(100, func() { cache.parse(input) }); allocs != 0 {
		t.Fatalf("warm inline lookup allocated %g objects", allocs)
	}
}
