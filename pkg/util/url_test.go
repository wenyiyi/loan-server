package util

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestDocumentURLs(t *testing.T) {
	for _, test := range []struct {
		name, url string
		valid     bool
	}{
		{"signed PDF", "https://files.example.com/a.pdf?signature=x", true},
		{"plain HTTP", "http://files.example.com/a.pdf", false},
		{"wrong file type", "https://files.example.com/a.jpg", false},
		{"embedded credentials", "https://user:pass@files.example.com/a.pdf", false},
		{"missing host", "https:///a.pdf", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Logf("[Input] case=%s, need HTTPS and .pdf; want valid=%t", test.name, test.valid)
			actual := ValidURL(test.url, ".pdf")
			t.Logf("[Actual] valid=%t, match=%t", actual, actual == test.valid)
			assert.Equal(t, test.valid, actual)
		})
	}
}
