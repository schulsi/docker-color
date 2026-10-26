package stdin

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
)

var ErrEmpty = errors.New("empty")

const capRows = 50

func Get(fn func(rows []string) error) error {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return fmt.Errorf("stdin: %w", err)
	} else if fi.Mode()&os.ModeNamedPipe == 0 && fi.Size() <= 0 {
		return fmt.Errorf("stdin: %w", ErrEmpty)
	}

	return Stream(os.Stdin, os.Stdout, fn)
}

func Stream(input io.Reader, output io.Writer, fn func(rows []string) error) error {
	var (
		xClearToEnd           = []byte("\033[J")
		xClearToLineEnd       = []byte("\033[K")
		xClearToLineEndPadded = []byte(" \033[K")
		xMoveCursorHome       = []byte("\033[H")
	)

	var (
		rows    = make([]string, 0, capRows)
		scanner = bufio.NewScanner(input)
		err     error
	)

	for scanner.Scan() {
		row, reset, skip := processRow(scanner.Bytes(), output, xClearToEnd, xClearToLineEnd, xClearToLineEndPadded, xMoveCursorHome)
		if reset && len(rows) > 0 {
			if err = fn(rows); err != nil {
				return err
			}

			rows = rows[:0]
		}

		if skip {
			continue
		}

		rows = append(rows, string(row))
	}

	if err = scanner.Err(); err != nil {
		return fmt.Errorf("stdin: scan: %w", err)
	}

	return fn(rows)
}

func processRow(row []byte, output io.Writer, xClearToEnd, xClearToLineEnd, xClearToLineEndPadded, xMoveCursorHome []byte) ([]byte, bool, bool) {
	reset := false

	if rest, prefix, ok := cutPrefixAny(row, xClearToLineEnd, xClearToLineEndPadded); ok {
		_, _ = output.Write(prefix)
		row = rest
		if len(row) == 0 {
			return nil, false, true
		}
	}

	row = trimSuffixAny(row, xClearToLineEnd, xClearToLineEndPadded)

	if rest, ok := bytes.CutPrefix(row, xClearToEnd); ok {
		_, _ = output.Write(xClearToEnd)
		row = rest
	}

	if rest, ok := bytes.CutPrefix(row, xMoveCursorHome); ok {
		_, _ = output.Write(xMoveCursorHome)
		row = rest
		reset = true
	}

	row = bytes.TrimRight(row, " \t\r")
	if len(row) == 0 {
		return nil, reset, true
	}

	return row, reset, false
}

func cutPrefixAny(row []byte, prefixes ...[]byte) ([]byte, []byte, bool) {
	for _, prefix := range prefixes {
		if bytes.HasPrefix(row, prefix) {
			return row[len(prefix):], prefix, true
		}
	}

	return row, nil, false
}

func trimSuffixAny(row []byte, suffixes ...[]byte) []byte {
	for _, suffix := range suffixes {
		if bytes.HasSuffix(row, suffix) {
			return row[:len(row)-len(suffix)]
		}
	}

	return row
}
