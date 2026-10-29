// Package projectaudio provides an opt-in audio resource feature for project
// bundles. Importing it is the link boundary for audio decoders and playback
// backends; the project core itself has no audio dependency.
package projectaudio

import (
	"fmt"

	"github.com/diiyw/onix/audio"
	"github.com/diiyw/onix/project"
)

const resourceKind = "audio"

// Feature loads audio manifest entries with a statically selected decoder.
type Feature[D audio.Decoder] struct{}

func (Feature[D]) ConfigureResources(context project.ResourceLoadContext, entries project.ResourceEntries) error {
	var decoder D
	for _, entry := range entries.Audio {
		if entry.ID == "" || entry.Path == "" {
			return fmt.Errorf("resources.json audio entry requires id and path")
		}
		path, err := project.NormalizeResourcePath(entry.Path)
		if err != nil {
			return fmt.Errorf("audio %q path: %w", entry.ID, err)
		}
		entry.Path = path
		format, err := audio.FormatFromPath(path)
		if err != nil {
			return fmt.Errorf("audio %q: %w", entry.ID, err)
		}
		entry := entry
		if err := context.RegisterNamedResource(resourceKind, entry.ID, project.ResourceRegistration[*audio.Clip]{
			Load: func() (*audio.Clip, error) {
				source, err := context.OpenBlob(entry.Path)
				if err != nil {
					return nil, fmt.Errorf("open %s: %w", entry.Path, err)
				}
				return decoder.DecodeAudio(source, format, entry.Stream)
			},
			Dispose: (*audio.Clip).Close,
		}); err != nil {
			return err
		}
	}
	return nil
}

// Clip returns an audio resource installed by Feature.
func Clip(resources *project.Resources, id string) (*audio.Clip, error) {
	return resources.LoadNamedResource[*audio.Clip](resourceKind, id)
}
