package main

import (
	"io"
	"os"
	"sync"
)

type RotatingLogWriter struct {
	fileName   string
	mu         sync.Mutex
	cv         *sync.Cond
	file       *os.File
	numWriters int
}

func NewRotatingLogWriter(fileName string) (*RotatingLogWriter, error) {
	file, err := os.OpenFile(fileName, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		return nil, err
	}

	w := &RotatingLogWriter{
		fileName:   fileName,
		file:       file,
		numWriters: 0,
	}
	w.cv = sync.NewCond(&w.mu)
	return w, nil
}

func (w *RotatingLogWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	file := w.file
	w.numWriters++
	w.mu.Unlock()

	defer func() {
		w.mu.Lock()
		w.numWriters--
		if w.numWriters <= 0 {
			w.cv.Signal()
		}
		w.mu.Unlock()
	}()

	return file.Write(p)
}

func (w *RotatingLogWriter) Close() error {
	w.mu.Lock()
	defer func() {
		w.cv.Signal()
		w.mu.Unlock()
	}()

	for w.numWriters > 0 {
		w.cv.Wait()
	}

	err0 := w.file.Sync()
	err1 := w.file.Close()
	if err0 != nil {
		return err0
	}
	return err1
}

func (w *RotatingLogWriter) Rotate() error {
	newFile, err := os.OpenFile(w.fileName, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		return err
	}

	w.mu.Lock()
	defer func() {
		w.cv.Signal()
		w.mu.Unlock()
	}()

	for w.numWriters > 0 {
		w.cv.Wait()
	}

	oldFile := w.file
	w.file = newFile

	err0 := oldFile.Sync()
	err1 := oldFile.Close()
	if err0 != nil {
		return err0
	}
	return err1
}

var _ io.WriteCloser = (*RotatingLogWriter)(nil)
