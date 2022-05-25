package metainfo

import (
	"crypto/sha256"
	"io"
)

func GeneratePieces(r io.Reader, pieceLength int64, b []byte) ([]byte, error) {
	for {
		h := sha256.New()
		written, err := io.CopyN(h, r, pieceLength)
		if written > 0 {
			b = h.Sum(b)
		}
		if err == io.EOF {
			return b, nil
		}
		if err != nil {
			return b, err
		}
	}
}
