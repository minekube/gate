package crypto

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

func TestLineSplitter(t *testing.T) {
	const s10 = "1234567890"
	checkSplit(t, strings.Repeat(s10, 10), 76)
}

func checkSplit(t *testing.T, inS string, n int) {
	in1 := strings.NewReader(inS)
	var buf bytes.Buffer
	lsWriter := newLineSplitterWriter(76, []byte("\r\n"), &buf)
	_, err := io.Copy(lsWriter, in1)
	if err != nil {
		t.Error(err)
	}
	lines := strings.Split(buf.String(), "\r\n")
	for _, vSEP := range lines {
		v := strings.TrimRight(vSEP, "\r\n")
		//fmt.Println(hex.Dump([]byte(v)))
		if len(v) > n {
			t.Errorf("Line '%s', length %d, expected 0 .. %d\n", v, len(v), n)
		}
	}
}
