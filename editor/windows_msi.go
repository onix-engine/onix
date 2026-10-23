package editor

import (
	"bytes"
	"crypto/sha1"
	"encoding/binary"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

const (
	wixV3Namespace = "http://schemas.microsoft.com/wix/2006/wi"
	wixV4Namespace = "http://wixtoolset.org/schemas/v4/wxs"
)

type windowsInstallerToolKind uint8

const (
	windowsInstallerWiXV4 windowsInstallerToolKind = iota
	windowsInstallerWixl
	windowsInstallerWiXV3
)

type windowsInstallerTool struct {
	kind   windowsInstallerToolKind
	build  string
	linker string
}

type installerFile struct {
	id          string
	componentID string
	source      string
	executable  bool
}

type installerDirectory struct {
	id       string
	name     string
	children map[string]*installerDirectory
	files    []installerFile
}

func packageWindowsMSI(root, bundleDir, executable, output, appName, projectID string, progress func(BuildProgress)) (resultErr error) {
	tool, err := resolveWindowsInstallerTool(exec.LookPath)
	if err != nil {
		return err
	}
	schema := wixV3Namespace
	if tool.kind == windowsInstallerWiXV4 {
		schema = wixV4Namespace
	}
	source := filepath.Join(root, ".onix-installer.wxs")
	object := filepath.Join(root, ".onix-installer.wixobj")
	debugDatabase := strings.TrimSuffix(output, filepath.Ext(output)) + ".wixpdb"
	defer func() {
		resultErr = errors.Join(resultErr, removeBuildIntermediates(source, object, debugDatabase))
	}()
	if err := writeWixSource(source, bundleDir, executable, appName, deterministicUUID(projectID+"\x00"+appName), schema); err != nil {
		return fmt.Errorf("write Windows installer source: %w", err)
	}
	if err := os.Remove(output); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("replace existing MSI: %w", err)
	}
	architecture := windowsInstallerArchitecture(runtime.GOARCH)
	switch tool.kind {
	case windowsInstallerWiXV4:
		err = runCommandWithProgress(progress, root, os.Environ(), tool.build, "build", "-arch", architecture, "-o", output, source)
	case windowsInstallerWixl:
		err = runCommandWithProgress(progress, root, os.Environ(), tool.build, "--arch", architecture, "-o", output, source)
	case windowsInstallerWiXV3:
		if err = runCommandWithProgress(progress, root, os.Environ(), tool.build, "-nologo", "-arch", architecture, "-out", object, source); err == nil {
			err = runCommandWithProgress(progress, root, os.Environ(), tool.linker, "-nologo", "-out", output, object)
		}
	}
	if err != nil {
		return fmt.Errorf("package Windows MSI: %w", err)
	}
	return os.Chmod(output, 0o644)
}

func removeBuildIntermediates(names ...string) error {
	var result error
	for _, name := range names {
		if err := os.Remove(name); err != nil && !os.IsNotExist(err) {
			result = errors.Join(result, fmt.Errorf("remove build intermediate %s: %w", name, err))
		}
	}
	return result
}

func resolveWindowsInstallerTool(lookPath func(string) (string, error)) (windowsInstallerTool, error) {
	if command, err := lookPath("wix"); err == nil {
		return windowsInstallerTool{kind: windowsInstallerWiXV4, build: command}, nil
	}
	if command, err := lookPath("wixl"); err == nil {
		return windowsInstallerTool{kind: windowsInstallerWixl, build: command}, nil
	}
	candle, candleErr := lookPath("candle")
	light, lightErr := lookPath("light")
	if candleErr == nil && lightErr == nil {
		return windowsInstallerTool{kind: windowsInstallerWiXV3, build: candle, linker: light}, nil
	}
	return windowsInstallerTool{}, fmt.Errorf("Windows MSI packaging requires WiX v4 (wix), wixl, or WiX v3 (candle and light)")
}

func windowsInstallerArchitecture(goarch string) string {
	if goarch == "amd64" {
		return "x64"
	}
	return goarch
}

func deterministicUUID(value string) string {
	digest := sha1.Sum([]byte(value))
	var id [16]byte
	copy(id[:], digest[:16])
	id[6] = id[6]&0x0f | 0x50
	id[8] = id[8]&0x3f | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		binary.BigEndian.Uint32(id[0:4]),
		binary.BigEndian.Uint16(id[4:6]),
		binary.BigEndian.Uint16(id[6:8]),
		binary.BigEndian.Uint16(id[8:10]),
		id[10:16],
	)
}

func writeWixSource(output, bundleDir, executable, appName, upgradeCode, schema string) error {
	tree, components, err := buildInstallerTree(bundleDir, executable, appName)
	if err != nil {
		return err
	}
	executable, err = filepath.Abs(executable)
	if err != nil {
		return err
	}
	if _, err := os.Stat(executable); err != nil {
		return fmt.Errorf("stat installer executable: %w", err)
	}
	var data bytes.Buffer
	data.WriteString(xml.Header)
	encoder := xml.NewEncoder(&data)
	encoder.Indent("", "  ")
	wix := xml.StartElement{Name: xml.Name{Local: "Wix"}, Attr: []xml.Attr{{Name: xml.Name{Local: "xmlns"}, Value: schema}}}
	if err := encoder.EncodeToken(wix); err != nil {
		return err
	}
	if schema == wixV4Namespace {
		err = writeWixV4Package(encoder, tree, components, appName, upgradeCode)
	} else {
		err = writeWixV3Product(encoder, tree, components, appName, upgradeCode)
	}
	if err != nil {
		return err
	}
	if err := encoder.EncodeToken(wix.End()); err != nil {
		return err
	}
	if err := encoder.Flush(); err != nil {
		return err
	}
	data.WriteByte('\n')
	return writeAtomicOutput(output, func(writer io.Writer) error {
		_, err := writer.Write(data.Bytes())
		return err
	})
}

func buildInstallerTree(bundleDir, executable, appName string) (*installerDirectory, []string, error) {
	bundleDir, err := filepath.Abs(bundleDir)
	if err != nil {
		return nil, nil, err
	}
	executable, err = filepath.Abs(executable)
	if err != nil {
		return nil, nil, err
	}
	root := &installerDirectory{id: "INSTALLFOLDER", name: appName, children: make(map[string]*installerDirectory)}
	components := make([]string, 0)
	directoryIndex, fileIndex := 0, 0
	err = filepath.WalkDir(bundleDir, func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if entry.Type()&fs.ModeSymlink != 0 || !entry.Type().IsRegular() {
			return fmt.Errorf("unsupported installer file %s", name)
		}
		relative, err := filepath.Rel(bundleDir, name)
		if err != nil {
			return err
		}
		directory := root
		parts := splitRelativePath(filepath.Dir(relative))
		for _, part := range parts {
			child := directory.children[part]
			if child == nil {
				directoryIndex++
				child = &installerDirectory{id: fmt.Sprintf("D%d", directoryIndex), name: part, children: make(map[string]*installerDirectory)}
				directory.children[part] = child
			}
			directory = child
		}
		fileIndex++
		componentID := fmt.Sprintf("C%d", fileIndex)
		directory.files = append(directory.files, installerFile{
			id:          fmt.Sprintf("F%d", fileIndex),
			componentID: componentID,
			source:      name,
			executable:  name == executable,
		})
		components = append(components, componentID)
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	if len(components) == 0 {
		return nil, nil, fmt.Errorf("Windows installer source directory is empty")
	}
	return root, components, nil
}

func splitRelativePath(name string) []string {
	if name == "." || name == "" {
		return nil
	}
	result := make([]string, 0)
	for name != "." && name != "" {
		directory, base := filepath.Split(name)
		result = append(result, base)
		name = filepath.Clean(directory)
	}
	for left, right := 0, len(result)-1; left < right; left, right = left+1, right-1 {
		result[left], result[right] = result[right], result[left]
	}
	return result
}

func writeWixV3Product(encoder *xml.Encoder, tree *installerDirectory, components []string, appName, upgradeCode string) error {
	product := xmlElement("Product",
		xmlAttribute("Id", "*"), xmlAttribute("Name", appName), xmlAttribute("Language", "1033"),
		xmlAttribute("Version", "1.0.0"), xmlAttribute("Manufacturer", "Onix"), xmlAttribute("UpgradeCode", upgradeCode),
	)
	if err := encoder.EncodeToken(product); err != nil {
		return err
	}
	for _, element := range []xml.StartElement{
		xmlElement("Package", xmlAttribute("InstallerVersion", "500"), xmlAttribute("Compressed", "yes"), xmlAttribute("InstallScope", "perMachine")),
		xmlElement("MajorUpgrade", xmlAttribute("DowngradeErrorMessage", "A newer version of this application is already installed.")),
		xmlElement("MediaTemplate", xmlAttribute("EmbedCab", "yes")),
	} {
		if err := encodeEmptyElement(encoder, element); err != nil {
			return err
		}
	}
	target := xmlElement("Directory", xmlAttribute("Id", "TARGETDIR"), xmlAttribute("Name", "SourceDir"))
	programFiles := xmlElement("Directory", xmlAttribute("Id", "ProgramFiles64Folder"))
	if err := encoder.EncodeToken(target); err != nil {
		return err
	}
	if err := encoder.EncodeToken(programFiles); err != nil {
		return err
	}
	if err := writeInstallerDirectory(encoder, tree, appName, false); err != nil {
		return err
	}
	if err := encoder.EncodeToken(programFiles.End()); err != nil {
		return err
	}
	if err := writeStartMenuDirectory(encoder, appName, false); err != nil {
		return err
	}
	if err := encoder.EncodeToken(target.End()); err != nil {
		return err
	}
	if err := writeInstallerFeature(encoder, components, appName); err != nil {
		return err
	}
	return encoder.EncodeToken(product.End())
}

func writeWixV4Package(encoder *xml.Encoder, tree *installerDirectory, components []string, appName, upgradeCode string) error {
	installer := xmlElement("Package",
		xmlAttribute("Name", appName), xmlAttribute("Language", "1033"), xmlAttribute("Version", "1.0.0"),
		xmlAttribute("Manufacturer", "Onix"), xmlAttribute("UpgradeCode", upgradeCode),
		xmlAttribute("InstallerVersion", "500"), xmlAttribute("Scope", "perMachine"),
	)
	if err := encoder.EncodeToken(installer); err != nil {
		return err
	}
	for _, element := range []xml.StartElement{
		xmlElement("MajorUpgrade", xmlAttribute("DowngradeErrorMessage", "A newer version of this application is already installed.")),
		xmlElement("MediaTemplate", xmlAttribute("EmbedCab", "yes")),
	} {
		if err := encodeEmptyElement(encoder, element); err != nil {
			return err
		}
	}
	programFiles := xmlElement("StandardDirectory", xmlAttribute("Id", "ProgramFiles64Folder"))
	if err := encoder.EncodeToken(programFiles); err != nil {
		return err
	}
	if err := writeInstallerDirectory(encoder, tree, appName, true); err != nil {
		return err
	}
	if err := encoder.EncodeToken(programFiles.End()); err != nil {
		return err
	}
	if err := writeStartMenuDirectory(encoder, appName, true); err != nil {
		return err
	}
	if err := writeInstallerFeature(encoder, components, appName); err != nil {
		return err
	}
	return encoder.EncodeToken(installer.End())
}

func writeInstallerDirectory(encoder *xml.Encoder, directory *installerDirectory, appName string, v4 bool) error {
	start := xmlElement("Directory", xmlAttribute("Id", directory.id), xmlAttribute("Name", directory.name))
	if err := encoder.EncodeToken(start); err != nil {
		return err
	}
	for _, file := range directory.files {
		componentAttributes := []xml.Attr{xmlAttribute("Id", file.componentID), xmlAttribute("Guid", "*")}
		if v4 {
			componentAttributes = append(componentAttributes, xmlAttribute("Bitness", "always64"))
		} else {
			componentAttributes = append(componentAttributes, xmlAttribute("Win64", "yes"))
		}
		component := xmlElement("Component", componentAttributes...)
		if err := encoder.EncodeToken(component); err != nil {
			return err
		}
		fileElement := xmlElement("File", xmlAttribute("Id", file.id), xmlAttribute("Source", file.source), xmlAttribute("KeyPath", "yes"))
		if file.executable {
			if err := encoder.EncodeToken(fileElement); err != nil {
				return err
			}
			shortcut := xmlElement("Shortcut",
				xmlAttribute("Id", "ApplicationStartMenuShortcut"), xmlAttribute("Directory", "ApplicationProgramsFolder"),
				xmlAttribute("Name", appName), xmlAttribute("WorkingDirectory", "INSTALLFOLDER"), xmlAttribute("Advertise", "yes"),
			)
			if err := encodeEmptyElement(encoder, shortcut); err != nil {
				return err
			}
			if err := encoder.EncodeToken(fileElement.End()); err != nil {
				return err
			}
			if err := encodeEmptyElement(encoder, xmlElement("RemoveFolder", xmlAttribute("Id", "ApplicationProgramsFolderCleanup"), xmlAttribute("Directory", "ApplicationProgramsFolder"), xmlAttribute("On", "uninstall"))); err != nil {
				return err
			}
		} else if err := encodeEmptyElement(encoder, fileElement); err != nil {
			return err
		}
		if err := encoder.EncodeToken(component.End()); err != nil {
			return err
		}
	}
	children := make([]string, 0, len(directory.children))
	for name := range directory.children {
		children = append(children, name)
	}
	sort.Strings(children)
	for _, name := range children {
		if err := writeInstallerDirectory(encoder, directory.children[name], appName, v4); err != nil {
			return err
		}
	}
	return encoder.EncodeToken(start.End())
}

func writeStartMenuDirectory(encoder *xml.Encoder, appName string, v4 bool) error {
	containerName := "Directory"
	if v4 {
		containerName = "StandardDirectory"
	}
	container := xmlElement(containerName, xmlAttribute("Id", "ProgramMenuFolder"))
	programs := xmlElement("Directory", xmlAttribute("Id", "ApplicationProgramsFolder"), xmlAttribute("Name", appName))
	if err := encoder.EncodeToken(container); err != nil {
		return err
	}
	if err := encodeEmptyElement(encoder, programs); err != nil {
		return err
	}
	return encoder.EncodeToken(container.End())
}

func writeInstallerFeature(encoder *xml.Encoder, components []string, appName string) error {
	feature := xmlElement("Feature", xmlAttribute("Id", "MainFeature"), xmlAttribute("Title", appName), xmlAttribute("Level", "1"))
	if err := encoder.EncodeToken(feature); err != nil {
		return err
	}
	for _, component := range components {
		if err := encodeEmptyElement(encoder, xmlElement("ComponentRef", xmlAttribute("Id", component))); err != nil {
			return err
		}
	}
	return encoder.EncodeToken(feature.End())
}

func xmlElement(name string, attributes ...xml.Attr) xml.StartElement {
	return xml.StartElement{Name: xml.Name{Local: name}, Attr: attributes}
}

func xmlAttribute(name, value string) xml.Attr {
	return xml.Attr{Name: xml.Name{Local: name}, Value: value}
}

func encodeEmptyElement(encoder *xml.Encoder, element xml.StartElement) error {
	if err := encoder.EncodeToken(element); err != nil {
		return err
	}
	return encoder.EncodeToken(element.End())
}
