package metrics

import (
	"io"
	"os"
	"strings"
	"syscall"
)

// tailer follows log files and hands each new line to a callback. It keeps
// the file open, so lines written just before a rotation are still read,
// and notices rotation (new inode) and truncation (copytruncate).
//
// Files that exist when the first round of polling happens are followed
// from their end, so a restart does not count old traffic again; files that
// appear later are read from the start.
type tailer struct {
	files  map[string]*followed
	primed bool
}

type followed struct {
	f       *os.File
	ino     uint64
	partial string
}

func newTailer() *tailer { return &tailer{files: map[string]*followed{}} }

func inode(fi os.FileInfo) uint64 {
	if st, ok := fi.Sys().(*syscall.Stat_t); ok {
		return st.Ino
	}
	return 0
}

// prime ends the first round: files found from now on are read from the start.
func (t *tailer) prime() { t.primed = true }

// poll reads what was appended to path since the last call.
func (t *tailer) poll(path string, fn func(line string)) {
	fl := t.files[path]
	if fl == nil {
		var ok bool
		if fl, ok = t.open(path, !t.primed); !ok {
			return
		}
		t.files[path] = fl
	}
	fl.drain(fn)
	// The path now names a different file: the old one was rotated away and
	// is fully read, so move on to the new one.
	if fi, err := os.Stat(path); err == nil && inode(fi) != fl.ino {
		fl.f.Close()
		delete(t.files, path)
		if nf, ok := t.open(path, false); ok {
			t.files[path] = nf
			nf.drain(fn)
		}
	}
}

func (t *tailer) open(path string, toEnd bool) (*followed, bool) {
	f, err := os.Open(path)
	if err != nil {
		return nil, false
	}
	fi, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, false
	}
	if toEnd {
		f.Seek(0, io.SeekEnd)
	}
	return &followed{f: f, ino: inode(fi)}, true
}

func (fl *followed) drain(fn func(string)) {
	if fi, err := fl.f.Stat(); err == nil {
		if pos, _ := fl.f.Seek(0, io.SeekCurrent); fi.Size() < pos {
			fl.f.Seek(0, io.SeekStart) // truncated in place
			fl.partial = ""
		}
	}
	buf := make([]byte, 64<<10)
	for {
		n, err := fl.f.Read(buf)
		if n > 0 {
			data := fl.partial + string(buf[:n])
			lines := strings.Split(data, "\n")
			fl.partial = lines[len(lines)-1]
			for _, l := range lines[:len(lines)-1] {
				if l != "" {
					fn(l)
				}
			}
		}
		if err != nil || n == 0 {
			return
		}
	}
}

// keep closes the files not in paths (hosts that were deleted or filtered out).
func (t *tailer) keep(paths map[string]bool) {
	for p, fl := range t.files {
		if !paths[p] {
			fl.f.Close()
			delete(t.files, p)
		}
	}
}

func (t *tailer) close() {
	t.keep(nil)
}
