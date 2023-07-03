package favicon

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	_ "image/jpeg"
	"image/png"
	"os"
	"strings"

	"github.com/nfnt/resize"
	"gopkg.in/yaml.v3"
)

// Favicon is 64x64 sized data uri image send in response to a server list ping.
// Refer to https://en.wikipedia.org/wiki/Data_URI_scheme for details.
// Example: "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAEAAAAABCAYAAABubagXAAAAEElEQVR42mP8z8BQzzCCAQB+lAGA+H8KEAAAAABJRU5ErkJggg=="
type Favicon string

// Make sure Favicon implements the interfaces at compile time.
var (
	_ yaml.Unmarshaler = (*Favicon)(nil)
	_ json.Unmarshaler = (*Favicon)(nil)
)

// UnmarshalJSON implements json.Unmarshaler.
func (f *Favicon) UnmarshalJSON(bytes []byte) (err error) {
	var s string
	if err := json.Unmarshal(bytes, &s); err != nil {
		return err
	}
	*f, err = Parse(s)
	return err
}

// UnmarshalYAML implements yaml.Unmarshaler.
func (f *Favicon) UnmarshalYAML(value *yaml.Node) (err error) {
	var s string
	if err = value.Decode(&s); err != nil {
		return err
	}
	*f, err = Parse(s)
	return err
}

// FromImage converts an image.Image to Favicon.
func FromImage(img image.Image) (Favicon, error) {
	// Resize down to 64x64 if necessary
	if img.Bounds().Max.X > 64 || img.Bounds().Max.Y > 64 {
		img = resize.Resize(64, 64, img, resize.NearestNeighbor)
	}

	buf := new(bytes.Buffer)
	if err := png.Encode(buf, img); err != nil {
		return "", err
	}
	return FromBytes(buf.Bytes()), nil
}

// FromFile takes the filename of an image and converts it to Favicon.
func FromFile(filename string) (Favicon, error) {
	f, err := os.Open(filename)
	if err != nil {
		return "", err
	}
	defer f.Close()

	img, _, err := image.Decode(f)
	if err != nil {
		return "", err
	}
	return FromImage(img)
}

const (
	dataImagePrefix = "data:image/"
	dataFullPrefix  = dataImagePrefix + "png;base64,"
)

// Parse takes a data uri string or filename and converts it to Favicon.
func Parse(s string) (Favicon, error) {
	if strings.HasPrefix(s, dataImagePrefix) {
		return Favicon(s), nil
	}
	if stat, err := os.Stat(s); err == nil && !stat.IsDir() {
		f, err := FromFile(s)
		if err != nil {
			return "", fmt.Errorf("favicon: %w", err)
		}
		return f, nil
	}
	return "", fmt.Errorf("favicon: invalid format or file not found: %s", s)
}

// FromBytes takes base64 bytes encoding of an image and converts it to Favicon.
func FromBytes(b []byte) Favicon {
	b = bytes.TrimPrefix(b, []byte(dataFullPrefix))
	b64 := base64.StdEncoding.EncodeToString(b)
	return Favicon(dataFullPrefix + b64)
}

// Bytes returns the bytes encoding of the favicon.
func (f Favicon) Bytes() []byte {
	s := strings.TrimPrefix(string(f), dataFullPrefix)
	b, _ := base64.StdEncoding.DecodeString(s)
	return b
}
