package od

// This file regroups OD extensions that are executed when reading or writing to object dictionary

import (
	"io"
	"log/slog"
	"os"
)

type FileObject struct {
	logger    *slog.Logger
	FilePath  string
	WriteMode int
	ReadMode  int
	File      *os.File
}

func NewFileObject(path string, logger *slog.Logger, writeMode int, readMode int) *FileObject {

	if logger == nil {
		logger = slog.Default()
	}

	return &FileObject{
		logger:    logger.With("extension", "[FILE]"),
		FilePath:  path,
		WriteMode: writeMode,
		ReadMode:  readMode}
}

// [SDO] Custom function for reading a file like object
func ReadEntryFileObject(stream *Stream, data []byte) (uint16, error) {
	if stream == nil || data == nil || stream.Subindex != 0 || stream.Object == nil {
		return 0, ErrDevIncompat
	}
	fileObject, ok := stream.Object.(*FileObject)
	if !ok {
		stream.DataOffset = 0
		return 0, ErrDevIncompat
	}
	if stream.DataOffset == 0 {
		var err error
		fileObject.logger.Info("opening file for reading", "path", fileObject.FilePath)
		fileObject.File, err = os.OpenFile(fileObject.FilePath, fileObject.ReadMode, 0644)
		if err != nil {
			return 0, ErrDevIncompat
		}
	} else {
		// Re-adjust file cursor depending on datoffset
		_, err := fileObject.File.Seek(int64(stream.DataOffset), 0)
		if err != nil {
			return 0, ErrDevIncompat
		}
	}
	countReadInt, err := io.ReadFull(fileObject.File, data)

	switch err {
	case nil:
		stream.DataOffset += uint32(countReadInt)
		return uint16(countReadInt), ErrPartial
	case io.EOF, io.ErrUnexpectedEOF:
		fileObject.logger.Info("finished reading", "path", fileObject.FilePath)
		fileObject.File.Close()
		return uint16(countReadInt), nil
	default:
		// unexpected error
		fileObject.logger.Warn("error reading", "path", fileObject.FilePath, "err", err)
		fileObject.File.Close()
		return uint16(countReadInt), ErrDevIncompat
	}
}

// [SDO] Custom function for writing a file like object
func WriteEntryFileObject(stream *Stream, data []byte) (uint16, error) {
	if stream == nil || data == nil || stream.Subindex != 0 || stream.Object == nil {
		return 0, ErrDevIncompat
	}
	fileObject, ok := stream.Object.(*FileObject)
	if !ok {
		stream.DataOffset = 0
		return 0, ErrDevIncompat
	}
	if stream.DataOffset == 0 {
		var err error
		fileObject.logger.Info("opening file for writing", "path", fileObject.FilePath)
		fileObject.File, err = os.OpenFile(fileObject.FilePath, fileObject.WriteMode, 0644)
		if err != nil {
			return 0, ErrDevIncompat
		}
	} else {
		// Re-adjust file cursor depending on datoffset
		_, err := fileObject.File.Seek(int64(stream.DataOffset), 0)
		if err != nil {
			return 0, ErrDevIncompat
		}
	}

	countWrittenInt, err := fileObject.File.Write(data)
	if err == nil {
		stream.DataOffset += uint32(countWrittenInt)
		if stream.DataLength == stream.DataOffset {
			fileObject.logger.Info("finished writing", "path", fileObject.FilePath)
			fileObject.File.Close()
			return uint16(countWrittenInt), nil
		} else {
			return uint16(countWrittenInt), ErrPartial
		}
	} else {
		fileObject.logger.Warn("error writing", "path", fileObject.FilePath, "err", err)
		fileObject.File.Close()
		return uint16(countWrittenInt), ErrDevIncompat
	}
}

// [SDO] Custom function for reading an io.Reader
func ReadEntryReader(stream *Stream, data []byte) (uint16, error) {
	if stream == nil || data == nil || stream.Subindex != 0 || stream.Object == nil {
		return 0, ErrDevIncompat
	}
	reader, ok := stream.Object.(io.ReadSeeker)
	if !ok {
		stream.DataOffset = 0
		return 0, ErrDevIncompat
	}
	// If first read, go back to initial point
	if stream.DataOffset == 0 {
		_, err := reader.Seek(0, io.SeekStart)
		if err != nil {
			return 0, ErrDevIncompat
		}
	}
	// Read len(data) bytes
	countReadInt, err := io.ReadFull(reader, data)
	switch err {
	case nil:
		// Not finished reading
		stream.DataOffset += uint32(countReadInt)
		return uint16(countReadInt), ErrPartial
	case io.EOF, io.ErrUnexpectedEOF:
		return uint16(countReadInt), nil
	default:
		return uint16(countReadInt), ErrDevIncompat
	}
}
