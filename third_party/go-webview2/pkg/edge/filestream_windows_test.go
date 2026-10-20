//go:build windows

package edge

import (
	"bytes"
	"io"
	"os"
	"testing"
	"unsafe"
)

func TestFileStreamWriteReadAndDelete(t *testing.T) {
	stream, err := NewFileStream()
	if err != nil {
		t.Fatal(err)
	}
	path := stream.path
	defer stream.Close()
	body := bytes.Repeat([]byte("dictionary media"), 1<<18)
	if _, err := stream.Write(body); err != nil {
		t.Fatal(err)
	}
	if _, err := stream.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(stream)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, body) {
		t.Fatalf("read %d bytes, want %d matching bytes", len(got), len(body))
	}
	if err := stream.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("temporary stream file remains: %v", err)
	}
}

func TestFileStreamDeletesAfterLastCOMReference(t *testing.T) {
	stream, err := NewFileStream()
	if err != nil {
		t.Fatal(err)
	}
	path, ptr := stream.path, stream.ptr
	if _, err := stream.Write([]byte("retained content")); err != nil {
		t.Fatal(err)
	}
	if _, err := stream.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	com := (*fileStreamCOM)(unsafe.Pointer(ptr))
	_, _, _ = com.vtbl.AddRef.Call(ptr)
	if err := stream.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("temporary path remains after handing off the stream: %v", err)
	}
	var count uint32
	buffer := make([]byte, 16)
	hr, _, _ := com.vtbl.Read.Call(ptr, uintptr(unsafe.Pointer(&buffer[0])), uintptr(len(buffer)), uintptr(unsafe.Pointer(&count)))
	if int32(hr) < 0 || string(buffer[:count]) != "retained content" {
		t.Fatalf("retained stream unreadable: HRESULT=%#x, data=%q", hr, buffer[:count])
	}
	_, _, _ = com.vtbl.Release.Call(ptr)
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("file remains after the last COM reference: %v", err)
	}
}
