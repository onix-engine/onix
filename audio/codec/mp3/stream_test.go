package mp3

import (
	"bytes"
	"embed"
	"encoding/binary"
	"errors"
	"io"
	"io/fs"
	"math"
	"testing"
	"testing/synctest"

	"github.com/onix-engine/onix/audio"
	"github.com/onix-engine/onix/audio/codec/mp3/internal/layer3"
)

// Original half-second sweeps, distinct channel tones and noise bursts, encoded
// with LAME. The independent minimp3 reference PCM contains both channels of
// every 113th source frame, rounded to little-endian signed 16-bit samples.
//
//go:embed testdata/*
var fixtures embed.FS

func fixture(t testing.TB, name string) []byte {
	t.Helper()
	data, err := fixtures.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func BenchmarkDecodeMP3(b *testing.B) {
	data := bytes.Repeat(fixture(b, "stereo-vbr.mp3"), 8)
	b.ReportAllocs()
	for b.Loop() {
		if _, _, err := layer3.Decode(data); err != nil {
			b.Fatal(err)
		}
	}
}

func TestDecodeMP3(t *testing.T) {
	for _, tc := range []struct {
		name         string
		rate, frames int
		mono         bool
	}{
		{"stereo-vbr", 44100, 24192, false},
		{"mono", 22050, 12672, true},
		{"mpeg25", 8000, 5184, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data := fixture(t, tc.name+".mp3")
			want := fixture(t, tc.name+".s16le")
			decoded, err := decode(data)
			if err != nil {
				t.Fatal(err)
			}
			if decoded.SampleRate != tc.rate || decoded.Channels != 2 || len(decoded.Samples) != tc.frames*2 {
				t.Fatalf("decoded %d samples at %d Hz, %d channels", len(decoded.Samples), decoded.SampleRate, decoded.Channels)
			}
			if len(want) != ((tc.frames+112)/113)*4 {
				t.Fatal("incorrect reference length")
			}
			for i := 0; i < len(want); i += 2 {
				frame, ch := i/4*113, i/2%2
				sample := decoded.Samples[2*frame+ch]
				ref := float32(int16(binary.LittleEndian.Uint16(want[i:]))) / 32768
				if !(math.Abs(float64(sample-ref)) <= 0.51/32768) {
					t.Fatalf("frame %d channel %d = %g, reference %g", frame, ch, sample, ref)
				}
			}
			if tc.mono {
				for i := 0; i < len(decoded.Samples); i += 2 {
					if decoded.Samples[i] != decoded.Samples[i+1] {
						t.Fatalf("mono channels differ at %d", i/2)
					}
				}
			}
			staticSource := audio.NewMemorySource(data)
			static, err := (Decoder{}).DecodeAudio(staticSource, audio.FormatMP3, false)
			if err != nil {
				t.Fatal(err)
			}
			defer static.Close()
			if staticSource.Size() != 0 {
				t.Fatal("static source still open")
			}
			source := &streamOnlySource{MemorySource: audio.NewMemorySource(data)}
			clip, err := (Decoder{}).DecodeAudio(source, audio.FormatMP3, true)
			if err != nil {
				t.Fatal(err)
			}
			if !clip.Streaming() || clip.Duration() != static.Duration() || source.Size() == 0 {
				t.Fatalf("streaming = %t, duration %v, static duration %v", clip.Streaming(), clip.Duration(), static.Duration())
			}
			if err := clip.Close(); err != nil || source.Size() != 0 {
				t.Fatalf("close: %v, remaining source size %d", err, source.Size())
			}
		})
	}
}

type streamOnlySource struct{ *audio.MemorySource }

func (*streamOnlySource) Bytes() ([]byte, error) {
	return nil, errors.New("unexpected full-source read")
}

func openTestStream(t *testing.T, track *layer3.Track, data []byte) *streamReader {
	t.Helper()
	r, err := audio.NewMemorySource(data).Open()
	if err != nil {
		t.Fatal(err)
	}
	stream := newStreamReader(track, r)
	t.Cleanup(func() { _ = stream.Close() })
	return stream
}

func TestStreamReadAndSeek(t *testing.T) {
	for _, name := range []string{"stereo-vbr", "mono", "mpeg25"} {
		t.Run(name, func(t *testing.T) {
			data := fixture(t, name+".mp3")
			decoded, err := decode(data)
			if err != nil {
				t.Fatal(err)
			}
			track, err := layer3.Inspect(bytes.NewReader(data))
			if err != nil {
				t.Fatal(err)
			}
			stream := openTestStream(t, track, data)
			frames := audio.ResampledFrameCount(int64(len(decoded.Samples)/2), decoded.SampleRate)
			pcm := make([]byte, frames*audio.BytesPerFrame)
			// 997-byte reads split both float samples and compressed frames.
			for start := 0; start < len(pcm); start += 997 {
				chunk := pcm[start:min(start+997, len(pcm))]
				if n, err := stream.Read(chunk); n != len(chunk) || err != nil {
					t.Fatalf("read at %d returned %d, %v", start, n, err)
				}
			}
			for i := range int(frames) {
				position := float64(i) * float64(decoded.SampleRate) / audio.OutputSampleRate
				left := min(int(position), len(decoded.Samples)/2-1)
				right := min(left+1, len(decoded.Samples)/2-1)
				for ch := range 2 {
					a, b := decoded.Samples[2*left+ch], decoded.Samples[2*right+ch]
					want := a + (b-a)*float32(position-float64(left))
					got := math.Float32frombits(binary.LittleEndian.Uint32(pcm[(i*2+ch)*4:]))
					if got != want {
						t.Fatalf("resampled frame %d channel %d = %g, want %g", i, ch, got, want)
					}
				}
			}
			var block [383]byte
			if n, err := stream.Read(block[:]); n != 0 || err != io.EOF {
				t.Fatalf("end read = %d, %v", n, err)
			}
			// Both players share the index and seek independently across packets.
			second := openTestStream(t, track, data)
			for i, offset := range []int{len(pcm) - 501, 7, len(pcm)/3 + 5, len(pcm) / 2, 0, len(pcm) - 1377} {
				for j, player := range []*streamReader{stream, second} {
					at := int64(offset + i*j)
					if got, err := player.Seek(at, io.SeekStart); got != at || err != nil {
						t.Fatalf("seek to %d = %d, %v", at, got, err)
					}
					if _, err := io.ReadFull(player, block[:]); err != nil || !bytes.Equal(pcm[at:at+int64(len(block))], block[:]) {
						t.Fatalf("seek/read at %d differs from sequential PCM: %v", at, err)
					}
				}
			}
			if pos, err := second.Seek(-5, io.SeekEnd); err != nil || pos != int64(len(pcm)-5) {
				t.Fatalf("end-relative seek = %d, %v", pos, err)
			}
			if pos, err := second.Seek(math.MaxInt64, io.SeekCurrent); err != nil || pos != int64(len(pcm)) {
				t.Fatalf("clamped seek = %d, %v", pos, err)
			}
			if _, err := second.Seek(math.MinInt64, io.SeekCurrent); err == nil {
				t.Fatal("accepted negative seek")
			}
			if err := second.Close(); err != nil {
				t.Fatal(err)
			}
			if _, err := second.Read(block[:]); !errors.Is(err, fs.ErrClosed) {
				t.Fatalf("read after close = %v", err)
			}
		})
	}
}

func TestStreamDecodeDoesNotAllocate(t *testing.T) {
	data := fixture(t, "stereo-vbr.mp3")
	track, err := layer3.Inspect(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	stream := openTestStream(t, track, data)
	var block [3 * 1152 * audio.BytesPerFrame]byte
	allocs := testing.AllocsPerRun(10, func() {
		if _, err := stream.Seek(stream.length/2, io.SeekStart); err != nil {
			t.Fatal(err)
		}
		if _, err := io.ReadFull(stream, block[:]); err != nil {
			t.Fatal(err)
		}
	})
	if allocs != 0 {
		t.Fatalf("seek and decode allocated %g objects", allocs)
	}
}

type countedReader struct {
	*bytes.Reader
	bytesRead int
}

type blockedEncodedReader struct {
	*bytes.Reader
	closed chan struct{}
}

func (r *blockedEncodedReader) Read([]byte) (int, error) { <-r.closed; return 0, fs.ErrClosed }
func (r *blockedEncodedReader) Close() error             { close(r.closed); return nil }

func TestStreamCloseInterruptsEncodedRead(t *testing.T) {
	data := fixture(t, "stereo-vbr.mp3")
	track, err := layer3.Inspect(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	synctest.Test(t, func(t *testing.T) {
		r := newStreamReader(track, &blockedEncodedReader{bytes.NewReader(data), make(chan struct{})})
		done := make(chan error, 1)
		go func() {
			var sample [audio.BytesPerFrame]byte
			_, err := r.Read(sample[:])
			done <- err
		}()
		synctest.Wait()
		if err := r.Close(); err != nil {
			t.Fatal(err)
		}
		if err := <-done; !errors.Is(err, fs.ErrClosed) {
			t.Fatalf("interrupted read returned %v", err)
		}
	})
}

func (r *countedReader) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	r.bytesRead += n
	return n, err
}

func (*countedReader) Close() error { return nil }

func TestStreamSeekReadsOnlyNearbyPackets(t *testing.T) {
	data := bytes.Repeat(fixture(t, "stereo-vbr.mp3"), 16)
	track, err := layer3.Inspect(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	source := &countedReader{Reader: bytes.NewReader(data)}
	stream := newStreamReader(track, source)
	defer stream.Close()
	if _, err := stream.Seek(-1000, io.SeekEnd); err != nil {
		t.Fatal(err)
	}
	var frame [audio.BytesPerFrame]byte
	if _, err := io.ReadFull(stream, frame[:]); err != nil {
		t.Fatal(err)
	}
	if source.bytesRead > 8192 {
		t.Fatalf("seek near EOF read %d of %d encoded bytes", source.bytesRead, len(data))
	}
}

func TestMP3TagsAndIncompleteFrame(t *testing.T) {
	data := fixture(t, "stereo-vbr.mp3")
	// An ID3v2.4 tag containing an actual frame must be skipped, including its footer.
	tag := []byte{'I', 'D', '3', 4, 0, 0x10, 0, 0, 8, 20} // 1044 bytes of artwork
	tag = append(tag, data[:1044]...)
	tag = append(tag, []byte{'3', 'D', 'I', 4, 0, 0x10, 0, 0, 8, 20}...)
	tagged := append(tag, data...)
	trailer := make([]byte, 128)
	copy(trailer, "TAG")
	tagged = append(tagged, trailer...)
	tagged = append(tagged, data[:64]...) // incomplete trailing frame
	got, rate, err := layer3.Decode(tagged)
	if err != nil {
		t.Fatal(err)
	}
	want, _, err := layer3.Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	if rate != 44100 || len(got) != len(want) {
		t.Fatalf("tagged track: rate %d, samples %d", rate, len(got))
	}
	for i := range want {
		if want[i] != got[i] {
			t.Fatalf("tags changed sample %d", i)
		}
	}
	index, err := layer3.Inspect(bytes.NewReader(tagged))
	if err != nil || index.Frames() != int64(len(want)/2) {
		t.Fatalf("tagged index: %v, %v", index, err)
	}
	r := openTestStream(t, index, tagged)
	if _, err := r.Seek(r.length/2, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	var sample [8]byte
	if _, err := io.ReadFull(r, sample[:]); err != nil {
		t.Fatal(err)
	}
}

func TestDecodeErrorsCloseSource(t *testing.T) {
	data := fixture(t, "stereo-vbr.mp3")
	for _, stream := range []bool{false, true} {
		for _, bad := range [][]byte{nil, []byte("not an MP3"), data[:50], []byte("ID3\x04\x00\x00\x7f\x7f\x7f\x7f")} {
			source := audio.NewMemorySource(bad)
			if _, err := (Decoder{}).DecodeAudio(source, audio.FormatMP3, stream); err == nil || source.Size() != 0 {
				t.Fatalf("invalid MP3: stream %t, error %v, remaining source size %d", stream, err, source.Size())
			}
		}
		source := audio.NewMemorySource(data)
		if _, err := (Decoder{}).DecodeAudio(source, audio.FormatWAV, stream); err == nil || source.Size() != 0 {
			t.Fatalf("wrong format: stream %t, error %v", stream, err)
		}
	}
	// The first granule's big_values field cannot exceed 288 pairs.
	data[8], data[9] = 0xff, 0x80
	for _, stream := range []bool{false, true} {
		source := audio.NewMemorySource(data)
		if _, err := (Decoder{}).DecodeAudio(source, audio.FormatMP3, stream); err == nil || source.Size() != 0 {
			t.Fatalf("invalid Huffman region: stream %t, error %v", stream, err)
		}
	}
}
