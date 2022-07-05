package favicon

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	_ "image/jpeg"
	"image/png"
	"os"

	"github.com/nfnt/resize"
)

// Favicon is 64x64 sized data uri image send in response to a server list ping.
// Refer to https://en.wikipedia.org/wiki/Data_URI_scheme for details.
// Example: "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAEAAAAABCAYAAABubagXAAAAEElEQVR42mP8z8BQzzCCAQB+lAGA+H8KEAAAAABJRU5ErkJggg=="
type Favicon string

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
	b64 := base64.StdEncoding.EncodeToString(buf.Bytes())
	return Favicon(fmt.Sprintf("data:image/png;base64,%s", b64)), nil
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
