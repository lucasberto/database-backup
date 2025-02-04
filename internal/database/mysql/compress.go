package mysql

import (
	"bytes"
	"compress/gzip"

	"github.com/vbauerster/mpb/v8"
)

type CompressedProgressWriter struct {
	*ProgressWriter
	gzipWriter *gzip.Writer
	buffer     *bytes.Buffer
}

func NewCompressedProgressWriter(bar *mpb.Bar) *CompressedProgressWriter {
	buf := &bytes.Buffer{}
	pw := &ProgressWriter{
		Writer: buf,
		Bar:    bar,
	}

	return &CompressedProgressWriter{
		ProgressWriter: pw,
		gzipWriter:     gzip.NewWriter(pw),
		buffer:         buf,
	}
}

func (cpw *CompressedProgressWriter) Write(p []byte) (int, error) {
	return cpw.gzipWriter.Write(p)
}

func (cpw *CompressedProgressWriter) Close() error {
	if err := cpw.gzipWriter.Close(); err != nil {
		return err
	}
	return cpw.ProgressWriter.Close()
}

func (cpw *CompressedProgressWriter) Bytes() []byte {
	return cpw.buffer.Bytes()
}
