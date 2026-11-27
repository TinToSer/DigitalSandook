// Package audit is an append-only JSON-lines log of security-relevant events
// (logins, uploads, downloads, deletes, grants, admin actions).
// When configured with an HMAC key, each entry chains to the previous entry's
// MAC so tampering or line deletion is detectable.
package audit

import (
	"bufio"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
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
	MAC    string    `json:"mac,omitempty"`
}

type Log struct {
	mu      sync.Mutex
	path    string
	f       *os.File // nil after a failed reopen; retried on the next write
	size    int64
	key     []byte
	lastMAC string
}

func Open(dir string) (*Log, error) {
	l := &Log{path: filepath.Join(dir, "audit.log")}
	return l, l.open()
}

// SetKey enables HMAC hash-chaining on all subsequent log entries.
func (l *Log) SetKey(key []byte) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(key) == 0 {
		l.key = nil
		return
	}
	l.key = append([]byte(nil), key...)
	l.lastMAC = l.readLastMACLocked()
}

func (l *Log) readLastMACLocked() string {
	f, err := os.Open(l.path)
	if err != nil {
		return ""
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || st.Size() == 0 {
		return ""
	}
	const win = 64 << 10
	off := int64(0)
	if st.Size() > win {
		off = st.Size() - win
	}
	if _, err := f.Seek(off, io.SeekStart); err != nil {
		return ""
	}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 16<<10), 1<<20)
	var last string
	first := off > 0
	for sc.Scan() {
		if first {
			first = false
			continue
		}
		var e Entry
		if json.Unmarshal(sc.Bytes(), &e) == nil && e.MAC != "" {
			last = e.MAC
		}
	}
	return last
}

func computeMAC(key []byte, prev string, e Entry) string {
	m := hmac.New(sha256.New, key)
	m.Write([]byte(prev))
	m.Write([]byte{0})
	m.Write([]byte(e.T.UTC().Format(time.RFC3339Nano)))
	m.Write([]byte{0})
	m.Write([]byte(e.User))
	m.Write([]byte{0})
	m.Write([]byte(e.IP))
	m.Write([]byte{0})
	m.Write([]byte(e.Action))
	m.Write([]byte{0})
	m.Write([]byte(e.Detail))
	return hex.EncodeToString(m.Sum(nil))
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
	e := Entry{T: time.Now().UTC(), User: user, IP: ip, Action: action, Detail: detail}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.f != nil && l.size > rotateAt {
		l.f.Close()
		_ = os.Rename(l.path, l.path+".1")
		l.f = nil
		l.lastMAC = ""
	}
	if l.f == nil && l.open() != nil {
		return
	}
	if len(l.key) > 0 {
		e.MAC = computeMAC(l.key, l.lastMAC, e)
		l.lastMAC = e.MAC
	}
	b, _ := json.Marshal(e)
	b = append(b, '\n')
	n, _ := l.f.Write(b)
	l.size += int64(n)
}

// Verify checks the HMAC hash chain of the current audit.log file.
// Entries written before HMAC was enabled (MAC == "") are skipped until the
// first signed entry, after which every entry must chain cleanly.
func (l *Log) Verify() (checked int, valid bool, badLine int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.key) == 0 {
		return 0, true, 0
	}
	f, err := os.Open(l.path)
	if err != nil {
		return 0, true, 0
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	prev := ""
	seenMAC := false
	line := 0
	for sc.Scan() {
		line++
		var e Entry
		if err := json.Unmarshal(sc.Bytes(), &e); err != nil {
			return checked, false, line
		}
		if e.MAC == "" {
			if seenMAC {
				return checked, false, line
			}
			continue
		}
		seenMAC = true
		want := computeMAC(l.key, prev, e)
		if len(want) != len(e.MAC) || subtle.ConstantTimeCompare([]byte(want), []byte(e.MAC)) != 1 {
			return checked, false, line
		}
		prev = e.MAC
		checked++
	}
	return checked, true, 0
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
