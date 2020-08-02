package nbt

import (
	"fmt"
)

const (
	ListLimit       = 2097152
	ByteArrayLimit  = 16777216
	Int32ArrayLimit = ByteArrayLimit
	Int64ArrayLimit = ByteArrayLimit
	DepthLimit      = 512
)

type ErrorLimit struct {
	Subject string
	Length  int
	Limit   int
}

func (this ErrorLimit) Error() string {
	return fmt.Sprintf("Limit: %s %d > %d", this.Subject, this.Length, this.Limit)
}
