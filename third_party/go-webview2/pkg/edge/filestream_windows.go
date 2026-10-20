//go:build windows

package edge

import (
	"fmt"
	"io"
	"os"
	"syscall"
	"unsafe"

	"github.com/jchv/go-webview2/internal/w32"
)

const (
	stgmReadWrite       = 0x00000002
	stgmShareDenyNone   = 0x00000040
	stgmCreate          = 0x00001000
	stgmDeleteOnRelease = 0x04000000
)

type fileStreamVtbl struct {
	_IUnknownVtbl
	Read  ComProc
	Write ComProc
	Seek  ComProc
}

type fileStreamCOM struct {
	vtbl *fileStreamVtbl
}

// FileStream is a disk-backed COM IStream. It holds a temporary resource
// response without retaining its bytes in the Go heap.
type FileStream struct {
	ptr  uintptr
	path string
}

func NewFileStream() (*FileStream, error) {
	f, err := os.CreateTemp("", fmt.Sprintf("wudict2-webview-%d-*.tmp", os.Getpid()))
	if err != nil {
		return nil, err
	}
	path := f.Name()
	if err := f.Close(); err != nil {
		_ = os.Remove(path)
		return nil, err
	}
	ptr, err := w32.SHCreateStreamOnFileEx(path, stgmCreate|stgmReadWrite|stgmShareDenyNone|stgmDeleteOnRelease)
	if err != nil {
		_ = os.Remove(path)
		return nil, err
	}
	return &FileStream{ptr: ptr, path: path}, nil
}

func (s *FileStream) Pointer() uintptr { return s.ptr }

func (s *FileStream) Read(p []byte) (int, error) {
	if s.ptr == 0 {
		return 0, os.ErrClosed
	}
	if len(p) == 0 {
		return 0, nil
	}
	var read uint32
	stream := (*fileStreamCOM)(unsafe.Pointer(s.ptr))
	hr, _, _ := stream.vtbl.Read.Call(s.ptr, uintptr(unsafe.Pointer(&p[0])), uintptr(len(p)), uintptr(unsafe.Pointer(&read)))
	if int32(hr) < 0 {
		return int(read), syscall.Errno(hr)
	}
	if read == 0 {
		return 0, io.EOF
	}
	return int(read), nil
}

func (s *FileStream) Write(p []byte) (int, error) {
	if s.ptr == 0 {
		return 0, os.ErrClosed
	}
	if len(p) == 0 {
		return 0, nil
	}
	var written uint32
	stream := (*fileStreamCOM)(unsafe.Pointer(s.ptr))
	hr, _, _ := stream.vtbl.Write.Call(s.ptr, uintptr(unsafe.Pointer(&p[0])), uintptr(len(p)), uintptr(unsafe.Pointer(&written)))
	if int32(hr) < 0 {
		return int(written), syscall.Errno(hr)
	}
	if int(written) != len(p) {
		return int(written), io.ErrShortWrite
	}
	return int(written), nil
}

func (s *FileStream) Seek(offset int64, whence int) (int64, error) {
	if s.ptr == 0 {
		return 0, os.ErrClosed
	}
	var position uint64
	stream := (*fileStreamCOM)(unsafe.Pointer(s.ptr))
	hr, _, _ := stream.vtbl.Seek.Call(s.ptr, uintptr(offset), uintptr(whence), uintptr(unsafe.Pointer(&position)))
	if int32(hr) < 0 {
		return 0, syscall.Errno(hr)
	}
	return int64(position), nil
}

func (s *FileStream) Close() error {
	if s.ptr == 0 {
		return nil
	}
	stream := (*fileStreamCOM)(unsafe.Pointer(s.ptr))
	_, _, _ = stream.vtbl.Release.Call(s.ptr)
	s.ptr = 0
	// SHARE_DENY_NONE lets the name be removed while WebView2 retains and reads
	// the stream. The OS closes the underlying file after its final reference.
	_ = os.Remove(s.path)
	return nil
}
