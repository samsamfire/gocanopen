package od

import (
	"bytes"
	"io"
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
