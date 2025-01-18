package main

import (
	"net/url"
	"testing"
)

func TestShiftPath(t *testing.T) {
	tests := []struct{ Path, Head, Rest string }{
		{"/foo/", "foo", "/"},
		{"/foo/bar/", "foo", "/bar/"},
		{"/foo", "foo", "/"},
	}

	for _, test := range tests {
		url := &url.URL{Path: test.Path}
		head := shiftPath(url)
		if head != test.Head || url.Path != test.Rest {
			t.Errorf("Expected shiftPath('%s') = '%s', url = '%s'. Got '%s', '%s'",
				test.Path, test.Head, test.Rest, head, url.Path)
		}
	}
}
