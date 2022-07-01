package crypto

import "io"

// lineSplitter splits input every len bytes with a sep byte sequence, outputting to writer w
//
// See https://www.ietf.org/rfc/rfc2045.txt, section 6.8 for notes on maximum line length of 76 characters
//
// source https://github.com/tuck1s/go-smtpproxy/blob/04e888c69368fdf1c0a6ca166e19f47ee21b1ca6/linesplitter.go#L10
type lineSplitter struct {
	len   int
	count int
	sep   []byte
	w     io.Writer
}

// NewLineSplitterWriter creates a new instance
func newLineSplitterWriter(len int, sep []byte, w io.Writer) io.Writer {
	return &lineSplitter{len: len, count: 0, sep: sep, w: w}
}

// Write a line in to ls.len chunks with separator
func (ls *lineSplitter) Write(in []byte) (n int, err error) {
	writtenThisCall := 0
	readPos := 0
	// Leading chunk size is limited by: how much input there is; defined split length; and
	// any residual from last time
	chunkSize := min(len(in), ls.len-ls.count)
	// Pass on chunk(s)
	for {
		n, err = ls.w.Write(in[readPos:(readPos + chunkSize)])
		if err != nil {
			return n, err
		}
		readPos += chunkSize // Skip forward ready for next chunk
		ls.count += chunkSize
		writtenThisCall += chunkSize

		// if we have completed a chunk, emit a separator
		if ls.count >= ls.len {
			n, err = ls.w.Write(ls.sep)
			if err != nil {
				return n, err
			}
			// Don't increment writtenThisCall - io.Copy expects a count of bytes *copied* not written (otherwise raises a panic)
			ls.count = 0
		}
		inToGo := len(in) - readPos
		if inToGo <= 0 {
			break // reached end of input data
		}
		// Determine size of the NEXT chunk
		chunkSize = min(inToGo, ls.len)
	}
	return writtenThisCall, nil
}

// no min() built-in function for integers, so declare this here
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
