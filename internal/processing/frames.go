package processing

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// ExtractToZip preserves the FIAP starter's frame extraction cadence (fps=1).
// Each invocation uses an isolated temporary directory to avoid filename
// collisions when several videos are processed at the same time.
// ffmpegPath can be empty to resolve "ffmpeg" from PATH.
func ExtractToZip(ctx context.Context, ffmpegPath, videoPath, zipPath string) (int, error) {
	if strings.TrimSpace(videoPath) == "" || strings.TrimSpace(zipPath) == "" {
		return 0, fmt.Errorf("video and destination paths are required")
	}
	if ffmpegPath == "" {
		ffmpegPath = "ffmpeg"
	}
	if _, err := os.Stat(videoPath); err != nil {
		return 0, fmt.Errorf("video file: %w", err)
	}

	framesDir, err := os.MkdirTemp("", "fiapx-frames-*")
	if err != nil {
		return 0, fmt.Errorf("temporary directory: %w", err)
	}
	defer os.RemoveAll(framesDir)

	pattern := filepath.Join(framesDir, "frame_%06d.png")
	cmd := exec.CommandContext(ctx, ffmpegPath,
		"-hide_banner", "-nostdin", "-loglevel", "error",
		"-i", videoPath, "-vf", "fps=1", "-y", pattern)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return 0, fmt.Errorf("extract frames: %w: %s", err, strings.TrimSpace(string(output)))
	}

	frames, err := filepath.Glob(filepath.Join(framesDir, "*.png"))
	if err != nil {
		return 0, fmt.Errorf("list frames: %w", err)
	}
	if len(frames) == 0 {
		return 0, fmt.Errorf("video produced no frames")
	}
	sort.Strings(frames)
	if err := archiveFrames(frames, zipPath); err != nil {
		return 0, err
	}
	return len(frames), nil
}

func archiveFrames(frames []string, target string) (retErr error) {
	if len(frames) == 0 {
		return fmt.Errorf("no frames to archive")
	}
	file, err := os.Create(target)
	if err != nil {
		return fmt.Errorf("create zip: %w", err)
	}
	writer := zip.NewWriter(file)
	defer func() {
		if closeErr := writer.Close(); retErr == nil {
			retErr = closeErr
		}
		if closeErr := file.Close(); retErr == nil {
			retErr = closeErr
		}
		if retErr != nil {
			_ = os.Remove(target)
		}
	}()

	for _, frame := range frames {
		input, err := os.Open(frame)
		if err != nil {
			return fmt.Errorf("open frame: %w", err)
		}
		entry, err := writer.Create(filepath.Base(frame))
		if err != nil {
			_ = input.Close()
			return fmt.Errorf("create zip entry: %w", err)
		}
		_, copyErr := io.Copy(entry, input)
		closeErr := input.Close()
		if copyErr != nil {
			return fmt.Errorf("write zip entry: %w", copyErr)
		}
		if closeErr != nil {
			return fmt.Errorf("close frame: %w", closeErr)
		}
	}
	return nil
}
