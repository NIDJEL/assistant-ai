package domain

import "testing"

func TestZoomJoinURLsExcludeHostAndUntrustedURLs(t *testing.T) {
	for _, raw := range []string{"https://zoom.us/j/123?pwd=opaque", "https://us06web.zoom.us/j/456", "https://zoomgov.com/j/123"} {
		if !ValidZoomJoinURL(raw) {
			t.Error(raw)
		}
	}
	for _, raw := range []string{"https://zoom.us/s/123?zak=private", "https://zoom.us/start/123", "https://zoom.us.evil.test/j/123", "javascript:alert(1)", "https://zoom.us@evil.test/j/123", "https://zoom.us/j/123?zak=private", "http://zoom.us/j/123", "https://zoom.us:444/j/123"} {
		if ValidZoomJoinURL(raw) {
			t.Error(raw)
		}
	}
}
