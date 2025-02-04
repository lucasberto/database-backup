package mysql

import (
	"io"

	"github.com/vbauerster/mpb/v8"
)

type ProgressWriter struct {
	Writer io.Writer
	Bar    *mpb.Bar
}

func (pw *ProgressWriter) Write(p []byte) (int, error) {
	n, err := pw.Writer.Write(p)
	if err != nil {
		return n, err
	}
	pw.Bar.IncrBy(n)
	return n, nil
}

func (pw *ProgressWriter) Close() error {
	pw.Bar.SetTotal(-1, true)
	pw.Bar.SetCurrent(-1)
	return nil
}
