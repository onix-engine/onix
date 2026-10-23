#ifndef VULKAN_WIN32_H_
#define VULKAN_WIN32_H_ 1

/* Minimal VK_KHR_win32_surface declarations used by Onix. */
#define VK_KHR_win32_surface 1
#define VK_KHR_WIN32_SURFACE_SPEC_VERSION 6
#define VK_KHR_WIN32_SURFACE_EXTENSION_NAME "VK_KHR_win32_surface"

typedef VkFlags VkWin32SurfaceCreateFlagsKHR;

typedef struct VkWin32SurfaceCreateInfoKHR {
    VkStructureType sType;
    const void *pNext;
    VkWin32SurfaceCreateFlagsKHR flags;
    HINSTANCE hinstance;
    HWND hwnd;
} VkWin32SurfaceCreateInfoKHR;

typedef VkResult (VKAPI_PTR *PFN_vkCreateWin32SurfaceKHR)(
    VkInstance instance,
    const VkWin32SurfaceCreateInfoKHR *pCreateInfo,
    const VkAllocationCallbacks *pAllocator,
    VkSurfaceKHR *pSurface);

#ifndef VK_NO_PROTOTYPES
VKAPI_ATTR VkResult VKAPI_CALL vkCreateWin32SurfaceKHR(
    VkInstance instance,
    const VkWin32SurfaceCreateInfoKHR *pCreateInfo,
    const VkAllocationCallbacks *pAllocator,
    VkSurfaceKHR *pSurface);
#endif

/* Minimal external-memory declarations used for D3D11 texture import. */
#define VK_KHR_external_memory_win32 1
#define VK_KHR_EXTERNAL_MEMORY_WIN32_EXTENSION_NAME "VK_KHR_external_memory_win32"

typedef struct VkImportMemoryWin32HandleInfoKHR {
    VkStructureType sType;
    const void *pNext;
    VkExternalMemoryHandleTypeFlagBits handleType;
    HANDLE handle;
    LPCWSTR name;
} VkImportMemoryWin32HandleInfoKHR;

#define VK_KHR_win32_keyed_mutex 1
#define VK_KHR_WIN32_KEYED_MUTEX_EXTENSION_NAME "VK_KHR_win32_keyed_mutex"

typedef struct VkWin32KeyedMutexAcquireReleaseInfoKHR {
    VkStructureType sType;
    const void *pNext;
    uint32_t acquireCount;
    const VkDeviceMemory *pAcquireSyncs;
    const uint64_t *pAcquireKeys;
    const uint32_t *pAcquireTimeouts;
    uint32_t releaseCount;
    const VkDeviceMemory *pReleaseSyncs;
    const uint64_t *pReleaseKeys;
} VkWin32KeyedMutexAcquireReleaseInfoKHR;

#endif
