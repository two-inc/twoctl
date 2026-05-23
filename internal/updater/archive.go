package updater

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"strings"
)

// maxBinaryBytes bounds the extracted binary size. A gzip bomb would
// otherwise decompress unboundedly into the io.ReadAll call.
const maxBinaryBytes = 200 << 20 // 200 MiB

// extractFromTarGz returns a reader positioned at the `twoctl` binary inside
// a goreleaser-produced tar.gz archive. Rejects non-regular tar entries
// (symlinks, hard links, devices) and caps total bytes read.
func extractFromTarGz(r io.Reader) (io.Reader, error) {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return nil, err
	}
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil, errors.New("twoctl binary not found in archive")
		}
		if err != nil {
			return nil, err
		}
		if !isTwoctlBinary(hdr.Name) {
			continue
		}
		if hdr.Typeflag != tar.TypeReg {
			return nil, fmt.Errorf("archive entry %s is not a regular file (typeflag=%d) - refusing", hdr.Name, hdr.Typeflag)
		}
		if hdr.Size > maxBinaryBytes {
			return nil, fmt.Errorf("archive entry %s exceeds %d byte cap", hdr.Name, maxBinaryBytes)
		}
		buf, err := io.ReadAll(io.LimitReader(tr, maxBinaryBytes+1))
		if err != nil {
			return nil, err
		}
		if int64(len(buf)) > maxBinaryBytes {
			return nil, fmt.Errorf("archive entry %s decompressed past %d byte cap", hdr.Name, maxBinaryBytes)
		}
		return bytes.NewReader(buf), nil
	}
}

func extractFromZip(buf []byte) (io.Reader, error) {
	zr, err := zip.NewReader(bytes.NewReader(buf), int64(len(buf)))
	if err != nil {
		return nil, err
	}
	for _, f := range zr.File {
		if !isTwoctlBinary(f.Name) {
			continue
		}
		if f.UncompressedSize64 > maxBinaryBytes {
			return nil, fmt.Errorf("zip entry %s exceeds %d byte cap", f.Name, maxBinaryBytes)
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		out, err := io.ReadAll(io.LimitReader(rc, maxBinaryBytes+1))
		_ = rc.Close()
		if err != nil {
			return nil, err
		}
		if int64(len(out)) > maxBinaryBytes {
			return nil, fmt.Errorf("zip entry %s decompressed past %d byte cap", f.Name, maxBinaryBytes)
		}
		return bytes.NewReader(out), nil
	}
	return nil, errors.New("twoctl binary not found in archive")
}

func isTwoctlBinary(name string) bool {
	base := name
	if i := strings.LastIndexAny(name, "/\\"); i >= 0 {
		base = name[i+1:]
	}
	return base == "twoctl" || base == "twoctl.exe"
}
