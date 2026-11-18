package processing

import (
	"archive/zip"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func fakeExtractor(t *testing.T, script string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("FFmpeg shell fixture requires a POSIX shell")
	}
	path := filepath.Join(t.TempDir(), "ffmpeg")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nset -eu\n"+script+"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestExtractToZipWithControlledFFmpeg(t *testing.T) {
	ffmpeg := fakeExtractor(t, `
for argument in "$@"; do output="$argument"; done
directory="$(dirname "$output")"
printf 'png-one' > "$directory/frame_000002.png"
printf 'png-two' > "$directory/frame_000001.png"
`)
	video := filepath.Join(t.TempDir(), "video.mp4")
	if err := os.WriteFile(video, []byte("fake input"), 0600); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(t.TempDir(), "frames.zip")
	count, err := ExtractToZip(context.Background(), ffmpeg, video, destination)
	if err != nil || count != 2 {
		t.Fatalf("extract frames: %d %v", count, err)
	}
	archive, err := zip.OpenReader(destination)
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	if len(archive.File) != 2 || archive.File[0].Name != "frame_000001.png" ||
		archive.File[1].Name != "frame_000002.png" {
		t.Fatalf("unexpected archive contents: %+v", archive.File)
	}
}

func TestExtractToZipHandlesDecodeAndArchivalFailures(t *testing.T) {
	video := filepath.Join(t.TempDir(), "video.mp4")
	if err := os.WriteFile(video, []byte("test"), 0600); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name, script, expected string
	}{
		{"decode error", "echo decoder-failed >&2; exit 13", "decoder-failed"},
		{"no frames", "exit 0", "produced no frames"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ffmpeg := fakeExtractor(t, test.script)
			_, err := ExtractToZip(context.Background(), ffmpeg, video, filepath.Join(t.TempDir(), "output.zip"))
			if err == nil || !strings.Contains(err.Error(), test.expected) {
				t.Fatalf("expected %q, got %v", test.expected, err)
			}
		})
	}
	ffmpeg := fakeExtractor(t, `
for arg in "$@"; do output="$arg"; done
printf 'png' > "$(dirname "$output")/frame_000001.png"
`)
	_, err := ExtractToZip(context.Background(), ffmpeg, video, t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "create zip") {
		t.Fatalf("expected invalid ZIP destination error: %v", err)
	}
	_, err = ExtractToZip(context.Background(), ffmpeg, "", "result.zip")
	if err == nil || !strings.Contains(err.Error(), "required") {
		t.Fatalf("expected missing path validation: %v", err)
	}
	_, err = ExtractToZip(context.Background(), ffmpeg, video, "")
	if err == nil || !strings.Contains(err.Error(), "required") {
		t.Fatalf("expected missing destination validation: %v", err)
	}
}

func TestArchiveFramesRemovesPartialOutputOnMissingFrame(t *testing.T) {
	target := filepath.Join(t.TempDir(), "frames.zip")
	err := archiveFrames([]string{filepath.Join(t.TempDir(), "missing.png")}, target)
	if err == nil || !strings.Contains(err.Error(), "open frame") {
		t.Fatalf("expected source frame error: %v", err)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("partial ZIP artifact remained after failure: %v", err)
	}
}
