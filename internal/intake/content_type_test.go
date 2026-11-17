package intake

import "testing"

func TestVideoContentTypesAndExtensions(t *testing.T) {
	for _, item := range []struct {
		extension, mime string
		allowed         bool
	}{
		{".mp4", "video/mp4", true},
		{".mov", "video/quicktime", true},
		{".webm", "video/webm", true},
		{".avi", "video/x-msvideo", true},
		{".mkv", "application/octet-stream", true},
		{".exe", "application/octet-stream", false},
	} {
		if got := contentType(item.extension); got != item.mime {
			t.Errorf("%s: MIME = %s, want %s", item.extension, got, item.mime)
		}
		if got := allowedExtension(item.extension); got != item.allowed {
			t.Errorf("%s: allowed=%v, want %v", item.extension, got, item.allowed)
		}
	}
}
