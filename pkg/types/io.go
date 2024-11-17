// Copyright (c) 2024 TruthOS
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package types

import (
	"bytes"
	"context"
	"encoding/base64"
	"io"
	"mime"
	"os"
	"path/filepath"
	"strings"
)

// FileType represents supported file types
type FileType string

const (
	FileTypeText  FileType = "text"
	FileTypeImage FileType = "image"
	FileTypeAudio FileType = "audio"
	FileTypePDF   FileType = "pdf"
	FileTypeVideo FileType = "video"
)

// FileContent represents file data and metadata
type FileContent struct {
	Type     FileType
	Data     []byte
	MimeType string
	Name     string
	Size     int64
	Metadata map[string]interface{}
}

// FileReader reads and processes different file types
type FileReader struct {
	allowedBasePath string
	MaxSize         int64
	AllowedTypes    map[FileType][]string // map of file type to allowed extensions
}

// NewFileReader creates a new FileReader with default settings
func NewFileReader() *FileReader {
	return &FileReader{
		MaxSize: 100 * 1024 * 1024, // 100MB default
		AllowedTypes: map[FileType][]string{
			FileTypeText:  {".txt", ".md", ".json", ".yaml", ".yml"},
			FileTypeImage: {".png", ".jpg", ".jpeg", ".gif", ".webp"},
			FileTypeAudio: {".mp3", ".wav", ".ogg", ".m4a"},
			FileTypePDF:   {".pdf"},
			FileTypeVideo: {".mp4", ".webm", ".mov"},
		},
	}
}

// ReadFile reads and processes a file
func (fr *FileReader) ReadFile(ctx context.Context, path string) (*FileContent, error) {
	// Sanitize and validate path
	cleanPath := filepath.Clean(path)
	if !strings.HasPrefix(cleanPath, fr.allowedBasePath) {
		return nil, ErrInputf("path is outside allowed directory: %s", path)
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, ErrInputf("failed to stat file: %v", err)
	}

	if info.Size() > fr.MaxSize {
		return nil, ErrInputf("file too large: %d bytes (max %d)", info.Size(), fr.MaxSize)
	}

	// Determine file type
	ext := filepath.Ext(path)
	fileType, err := fr.determineFileType(ext)
	if err != nil {
		return nil, err
	}

	// Read file
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, ErrInputf("failed to read file: %v", err)
	}

	// Get MIME type
	mimeType := mime.TypeByExtension(ext)
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}

	return &FileContent{
		Type:     fileType,
		Data:     data,
		MimeType: mimeType,
		Name:     filepath.Base(path),
		Size:     info.Size(),
		Metadata: map[string]interface{}{
			"extension": ext,
			"modified":  info.ModTime(),
		},
	}, nil
}

// determineFileType identifies the file type based on extension
func (fr *FileReader) determineFileType(ext string) (FileType, error) {
	ext = filepath.Ext(ext)
	for fileType, extensions := range fr.AllowedTypes {
		for _, allowedExt := range extensions {
			if ext == allowedExt {
				return fileType, nil
			}
		}
	}
	return "", ErrInputf("unsupported file extension: %s", ext)
}

// ToBase64 converts file content to base64
func (fc *FileContent) ToBase64() string {
	return base64.StdEncoding.EncodeToString(fc.Data)
}

// Reader returns an io.Reader for the file content
func (fc *FileContent) Reader() io.Reader {
	return bytes.NewReader(fc.Data)
}

// String returns string representation for text files
func (fc *FileContent) String() string {
	if fc.Type != FileTypeText {
		return ""
	}
	return string(fc.Data)
}
