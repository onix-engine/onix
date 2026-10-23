// Package fs provides Onix's persistent virtual filesystem.
//
// Every operation uses io/fs paths: names are relative, slash-separated, and
// cannot escape the mounted root. On native platforms a root is a host
// directory. In browsers it is a namespace persisted in IndexedDB. In WeChat
// it is a directory below USER_DATA_PATH.
//
// WriteFile and Rename create missing destination directories. Remove only
// removes files or empty directories; RemoveAll removes complete directory
// trees. FileSystem implements io/fs.FS, ReadFileFS, ReadDirFS, and StatFS.
package fs
