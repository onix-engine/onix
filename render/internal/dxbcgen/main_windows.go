// Command dxbcgen compiles generated HLSL into builtin DirectX 11 bytecode.
package main

/*
#cgo LDFLAGS: -ld3dcompiler
#define COBJMACROS
#include <windows.h>
#include <d3dcompiler.h>
#include <stdlib.h>
static HRESULT compile(const void *source, size_t size, const char *entry, const char *profile,
                       ID3DBlob **code, ID3DBlob **errors) {
    return D3DCompile(source, size, NULL, NULL, NULL, entry, profile,
                      D3DCOMPILE_ENABLE_STRICTNESS | D3DCOMPILE_OPTIMIZATION_LEVEL3, 0, code, errors);
}
static void *blob_data(ID3DBlob *blob) { return ID3D10Blob_GetBufferPointer(blob); }
static size_t blob_size(ID3DBlob *blob) { return ID3D10Blob_GetBufferSize(blob); }
static void release_blob(ID3DBlob *blob) { if (blob) ID3D10Blob_Release(blob); }
*/
import "C"

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unsafe"
)

func main() {
	paths, err := filepath.Glob("render/shaders/directx_*.hlsl")
	check(err)
	for _, path := range paths {
		source, err := os.ReadFile(path)
		check(err)
		pipeline := "sprite"
		if strings.Contains(path, "shape") {
			pipeline = "shape"
		} else if strings.Contains(path, "mesh") || strings.Contains(path, "picking") {
			pipeline = "mesh"
		}
		stage, profile := "vertex", "vs_5_0"
		if strings.Contains(path, ".frag.") {
			stage, profile = "fragment", "ps_5_0"
		}
		entry, target := C.CString(pipeline+"_"+stage), C.CString(profile)
		var code, messages *C.ID3DBlob
		result := C.compile(unsafe.Pointer(&source[0]), C.size_t(len(source)), entry, target, &code, &messages)
		C.free(unsafe.Pointer(entry))
		C.free(unsafe.Pointer(target))
		if result < 0 {
			panic(fmt.Sprintf("%s: %s", path, C.GoString((*C.char)(C.blob_data(messages)))))
		}
		data := C.GoBytes(C.blob_data(code), C.int(C.blob_size(code)))
		C.release_blob(code)
		C.release_blob(messages)
		check(os.WriteFile(strings.TrimSuffix(path, ".hlsl")+".cso", data, 0644))
	}
}
func check(err error) {
	if err != nil {
		panic(err)
	}
}
