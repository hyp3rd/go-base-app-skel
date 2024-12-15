package output

import "io"

type WriteResult struct {
	Writer io.Writer
	Name   string
	Bytes  int
	Err    error
}
