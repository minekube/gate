package configutil

import (
	"gopkg.in/yaml.v3"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestURL_MarshalJSON(t *testing.T) {
	u := URL(url.URL{Scheme: "http", Host: "example.com", Path: "/path"})
	expected := `"http://example.com/path"`

	data, err := u.MarshalJSON()
	assert.NoError(t, err)
	assert.JSONEq(t, expected, string(data))
}

func TestURL_UnmarshalJSON(t *testing.T) {
	var u URL
	data := `"http://example.com/path"`

	err := u.UnmarshalJSON([]byte(data))
	assert.NoError(t, err)
	assert.Equal(t, "http", u.Scheme)
	assert.Equal(t, "example.com", u.Host)
	assert.Equal(t, "/path", u.Path)
}

func TestURL_MarshalYAML(t *testing.T) {
	u := URL(url.URL{Scheme: "http", Host: "example.com", Path: "/path"})
	expected := "http://example.com/path"

	data, err := u.MarshalYAML()
	assert.NoError(t, err)
	assert.Equal(t, expected, data)
}

func TestURL_UnmarshalYAML(t *testing.T) {
	var u URL
	data := "http://example.com/path"

	node := yaml.Node{Kind: yaml.ScalarNode, Value: data}
	err := u.UnmarshalYAML(&node)
	assert.NoError(t, err)
	assert.Equal(t, "http", u.Scheme)
	assert.Equal(t, "example.com", u.Host)
	assert.Equal(t, "/path", u.Path)
}

func TestDecode_NonEmptyString(t *testing.T) {
	testCases := []struct {
		input    string
		expected bool // true if we expect a non-empty URL, false otherwise
	}{
		{"http://example.com/path", true},
		{"https://example.com", true},
		{"", false}, // An empty input string should result in an error
		{"ftp://example.com", true},
		{"not-a-url", true}, // Invalid URL should not result in an error
	}

	for _, tc := range testCases {
		t.Run(tc.input, func(t *testing.T) {
			var u URL
			err := u.decode(tc.input)

			if tc.expected {
				assert.NoError(t, err)
				assert.NotEmpty(t, u.T().String(), "URL should not be empty")
			} else {
				assert.Error(t, err, "Expected an error for input: %s", tc.input)
			}
		})
	}
}
