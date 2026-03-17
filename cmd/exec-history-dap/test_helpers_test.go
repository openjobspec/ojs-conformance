package main

import (
	"bufio"
	"io"
)

// bufWriter wraps an io.Writer in a *bufio.Writer for tests; the
// emitter expects a *bufio.Writer for direct flush control.
func bufWriter(w io.Writer) *bufio.Writer { return bufio.NewWriter(w) }
