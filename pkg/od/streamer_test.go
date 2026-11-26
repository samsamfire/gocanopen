package od

import (
	"bytes"
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStreamer(t *testing.T) {
	od := createOD()
	entry := od.Index(0x3018)
	assert.NotNil(t, entry)
	// Test access to subindex > 1 for variable
	_, err := NewStreamer(entry, 1, true)
	assert.Equal(t, ErrSubNotExist, err)
	// Test that subindex 0 returns nil
	_, err = NewStreamer(entry, 0, true)
	assert.Nil(t, err)
	// Test access to subindex 0 of Record should return nil
	entry = od.Index(0x3030)
	_, err = NewStreamer(entry, 0, true)
	assert.Nil(t, err)
	// Test access to out of range subindex
	_, err = NewStreamer(entry, 10, true)
	assert.Equal(t, ErrSubNotExist, err)

}

func TestStreamerCopy(t *testing.T) {
	od := Default()
	entry1 := od.Index(0x1021)
	assert.NotNil(t, entry1)
	r, err := NewStreamer(entry1, 0, true)
	assert.Nil(t, err)
	buffer := bytes.NewBuffer(make([]byte, 1000))
	n, err := io.CopyN(buffer, &r, 1)
	assert.Nil(t, err)
	assert.EqualValues(t, 1, n)
}

func TestStreamerResetData(t *testing.T) {
	od := Default()
	entry := od.Index(0x1017)
	assert.NotNil(t, entry)
	streamer, err := NewStreamer(entry, 0, true)
	assert.Nil(t, err)
	assert.EqualValues(t, 2, streamer.DataLength)

	// DataLength describes the same buffer as Data, so resetting one
	// should never leave a stale length behind
	streamer.ResetData(1, 1)
	assert.EqualValues(t, 1, len(streamer.Data))
	assert.EqualValues(t, 1, streamer.DataOffset)
	assert.EqualValues(t, 1, streamer.DataLength)

	streamer.ResetData(0, 0xFF)
	assert.EqualValues(t, 0, len(streamer.Data))
	assert.EqualValues(t, 0xFF, streamer.DataOffset)
	assert.EqualValues(t, 0, streamer.DataLength)
}

// Reading an entry that does not fit inside the given buffer should be done
// in several chunks, each chunk starting where the previous one stopped
func TestReadEntryDefaultChunked(t *testing.T) {
	odict := Default()
	streamer, err := NewStreamer(odict.Index(0x2009), 0, true)
	assert.Nil(t, err)
	streamer.ResetData(1500, 0)
	for i := range streamer.Data {
		streamer.Data[i] = byte(i)
	}
	expected := make([]byte, 1500)
	copy(expected, streamer.Data)

	buffer := make([]byte, 1000)
	n := 0
	assert.NotPanics(t, func() { n, err = streamer.Read(buffer) })
	assert.Equal(t, ErrPartial, err)
	assert.Equal(t, 1000, n)
	assert.True(t, bytes.Equal(expected[:1000], buffer[:n]), "first chunk should be the beginning of the entry")

	assert.NotPanics(t, func() { n, err = streamer.Read(buffer) })
	assert.Nil(t, err)
	assert.Equal(t, 500, n)
	assert.True(t, bytes.Equal(expected[1000:], buffer[:n]), "second chunk should follow the first one")
}

// Writing an entry that is bigger than the given buffer should be done
// in several chunks, each chunk starting where the previous one stopped
func TestWriteEntryDefaultChunked(t *testing.T) {
	odict := Default()
	streamer, err := NewStreamer(odict.Index(0x2009), 0, true)
	assert.Nil(t, err)
	streamer.ResetData(1500, 0)

	firstChunk := bytes.Repeat([]byte{0xAA}, 1000)
	n, err := streamer.Write(firstChunk)
	assert.Equal(t, ErrPartial, err)
	assert.Equal(t, 1000, n)
	assert.True(t, bytes.Equal(firstChunk, streamer.Data[:1000]), "first chunk should be written at the beginning")

	secondChunk := bytes.Repeat([]byte{0xBB}, 500)
	n, err = streamer.Write(secondChunk)
	assert.Nil(t, err)
	assert.Equal(t, 500, n)
	assert.True(t, bytes.Equal(secondChunk, streamer.Data[1000:]), "second chunk should follow the first one")
}

func TestStreamerDomainWithoutExtension(t *testing.T) {
	od := NewOD()
	_, err := od.AddVariableType(0x2000, "domain", DOMAIN, AttributeSdoRw, "")
	assert.Nil(t, err)
	streamer, err := od.Streamer(0x2000, 0, false)
	assert.Nil(t, err)
	// Domain without extension is disabled, it must not panic
	_, err = streamer.Read(make([]byte, 10))
	assert.Equal(t, ErrUnsuppAccess, err)
	_, err = streamer.Write(make([]byte, 10))
	assert.Equal(t, ErrUnsuppAccess, err)
}

func TestFileObjectAbortedTransferClosesFile(t *testing.T) {
	path := t.TempDir() + "/file.bin"
	assert.Nil(t, os.WriteFile(path, make([]byte, 100), 0o644))
	od := NewOD()
	od.AddFile(0x2000, "file", path, os.O_RDONLY, os.O_CREATE|os.O_TRUNC|os.O_WRONLY)

	// Start a read but never finish it (e.g. SDO transfer aborted)
	streamer, err := od.Streamer(0x2000, 0, false)
	assert.Nil(t, err)
	_, err = streamer.Read(make([]byte, 10))
	assert.Equal(t, ErrPartial, err)
	first := streamer.Object.(*FileObject).File

	// A new transfer must close the previous file
	streamer, err = od.Streamer(0x2000, 0, false)
	assert.Nil(t, err)
	_, err = streamer.Read(make([]byte, 10))
	assert.Equal(t, ErrPartial, err)
	_, err = first.Stat()
	assert.ErrorIs(t, err, os.ErrClosed)

	// Same for writes
	streamer, err = od.Streamer(0x2000, 0, false)
	assert.Nil(t, err)
	_, err = streamer.Write(make([]byte, 10))
	assert.Equal(t, ErrPartial, err)
	second := streamer.Object.(*FileObject).File
	_, err = second.Stat()
	assert.Nil(t, err)
	streamer, err = od.Streamer(0x2000, 0, false)
	assert.Nil(t, err)
	_, err = streamer.Write(make([]byte, 10))
	assert.Equal(t, ErrPartial, err)
	_, err = second.Stat()
	assert.ErrorIs(t, err, os.ErrClosed)
}
