cmake_minimum_required(VERSION 3.22.1)
project(onixbridge C)

add_library(onix SHARED IMPORTED)
set_target_properties(onix PROPERTIES
    IMPORTED_LOCATION "${CMAKE_CURRENT_SOURCE_DIR}/../jniLibs/${ANDROID_ABI}/libonix.so"
    IMPORTED_NO_SONAME TRUE
)

add_library(onixbridge SHARED onixbridge.c)

find_library(android-lib android)
find_library(log-lib log)
find_library(vulkan-lib vulkan)
find_library(opensles-lib OpenSLES)

target_link_libraries(
    onixbridge
    onix
    ${android-lib}
    ${log-lib}
	${vulkan-lib}
    ${opensles-lib}
)
