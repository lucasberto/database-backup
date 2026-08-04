package mysql

import (
	"os"

	"github.com/vbauerster/mpb/v8"
)

type CompressedProgressWriter struct {
	*ProgressWriter
	file *os.File
}

func NewCompressedProgressWriter(bar *mpb.Bar, filePath string) (*CompressedProgressWriter, error) {
	file, err := os.Create(filePath)
	if err != nil {
		return nil, err
	}

	pw := &ProgressWriter{
		Writer: file,
		Bar:    bar,
	}

	return &CompressedProgressWriter{
		ProgressWriter: pw,
		file:           file,
	}, nil
}

func (cpw *CompressedProgressWriter) Write(p []byte) (int, error) {
	return cpw.ProgressWriter.Write(p)
}

func (cpw *CompressedProgressWriter) Close() error {
	if err := cpw.ProgressWriter.Close(); err != nil {
		cpw.file.Close()
		return err
	}
	return cpw.file.Close()
}
