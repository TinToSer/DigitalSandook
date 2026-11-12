// Package audit is an append-only JSON-lines log of security-relevant events
// (logins, uploads, downloads, deletes, grants, admin actions).
package audit

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const rotateAt = 20 << 20 // the log is rotated to audit.log.1 at 20 MB

type Entry struct {
	T      time.Time `json:"t"`
	User   string    `json:"user"`
	IP     string    `json:"ip"`
	Action string    `json:"action"`
	Detail string    `json:"detail"`
}

type Log struct {
	mu   sync.Mutex
	path string
	f    *os.File // nil after a failed reopen; retried on the next write
	size int64
}

func Open(dir string) (*Log, error) {
	l := &Log{path: filepath.Join(dir, "audit.log")}
	return l, l.open()
}

func (l *Log) open() error {
	f, err := os.OpenFile(l.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		l.f = nil
		return err
	}
	l.size = 0
	if st, err := f.Stat(); err == nil {
		l.size = st.Size()
	}
	l.f = f
	return nil
}

// Write appends one event. Entries are JSON-encoded, so user-controlled text
// (file names) can never forge extra lines.
func (l *Log) Write(user, ip, action, detail string) {
	b, _ := json.Marshal(Entry{T: time.Now().UTC(), User: user, IP: ip, Action: action, Detail: detail})
	b = append(b, '\n')
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.f != nil && l.size > rotateAt {
		l.f.Close()
		_ = os.Rename(l.path, l.path+".1")
		l.f = nil
	}
	if l.f == nil && l.open() != nil {
		return
	}
	n, _ := l.f.Write(b)
	l.size += int64(n)
}

// Tail returns up to max of the newest entries, newest first.
func (l *Log) Tail(max int) []Entry {
	l.mu.Lock() // also keeps rotation from renaming the file while it is open (Windows)
	defer l.mu.Unlock()
	f, err := os.Open(l.path)
	if err != nil {
		return nil
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil
	}
	const win = 512 << 10
	off := int64(0)
	if st.Size() > win {
		off = st.Size() - win
	}
	if _, err := f.Seek(off, io.SeekStart); err != nil {
		return nil
	}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	var all []Entry
	first := off > 0
	for sc.Scan() {
		if first { // possibly partial line
			first = false
			continue
		}
		var e Entry
		if json.Unmarshal(sc.Bytes(), &e) == nil {
			all = append(all, e)
		}
	}
	if len(all) > max {
		all = all[len(all)-max:]
	}
	for i, j := 0, len(all)-1; i < j; i, j = i+1, j-1 {
		all[i], all[j] = all[j], all[i]
	}
	return all
}

func (l *Log) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.f == nil {
		return nil
	}
	err := l.f.Close()
	l.f = nil
	return err
}
