package processing

import (
	"archive/zip"
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestArchiveFrames(t *testing.T) {
	directory := t.TempDir()
	first := filepath.Join(directory, "frame_000001.png")
	second := filepath.Join(directory, "frame_000002.png")
	if err := os.WriteFile(first, []byte("image-one"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(second, []byte("image-two"), 0600); err != nil {
		t.Fatal(err)
	}
	result := filepath.Join(directory, "frames.zip")
	if err := archiveFrames([]string{first, second}, result); err != nil {
		t.Fatalf("archiveFrames: %v", err)
	}
	archive, err := zip.OpenReader(result)
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	if len(archive.File) != 2 {
		t.Fatalf("wanted 2 archived frames, got %d", len(archive.File))
	}
	for i, expected := range []string{"frame_000001.png", "frame_000002.png"} {
		if archive.File[i].Name != expected {
			t.Fatalf("archive filename %q, want %q", archive.File[i].Name, expected)
		}
	}
}

func TestArchiveFramesRejectsEmptyInput(t *testing.T) {
	if err := archiveFrames(nil, filepath.Join(t.TempDir(), "frames.zip")); err == nil {
		t.Fatal("expected error for no frames")
	}
}

func TestExtractToZipRejectsMissingFile(t *testing.T) {
	dir := t.TempDir()
	_, err := ExtractToZip(context.Background(), "", filepath.Join(dir, "missing.mp4"), filepath.Join(dir, "frames.zip"))
	if err == nil {
		t.Fatal("expected missing video error")
	}
}
