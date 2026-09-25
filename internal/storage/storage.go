package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	"ledstrip/internal/led"
)

// Store — персистентность color.txt и admins.txt (форматы совместимы
// с Python-версией: "R G B" и ID построчно).
type Store struct {
	Dir        string
	SuperAdmin int64

	mu     sync.Mutex
	admins map[int64]struct{}
}

func New(dir string, superAdmin int64) *Store {
	s := &Store{Dir: dir, SuperAdmin: superAdmin, admins: map[int64]struct{}{}}
	_ = os.MkdirAll(dir, 0o755)
	s.loadAdmins()
	return s
}

// --- Цвет ---

func (s *Store) LoadColor() led.RGB {
	b, err := os.ReadFile(filepath.Join(s.Dir, "color.txt"))
	if err != nil {
		return led.Black
	}
	f := strings.Fields(string(b))
	if len(f) < 3 {
		return led.Black
	}
	v := make([]uint8, 3)
	for i := range v {
		n, err := strconv.Atoi(f[i])
		if err != nil || n < 0 || n > 255 {
			return led.Black
		}
		v[i] = uint8(n)
	}
	return led.RGB{R: v[0], G: v[1], B: v[2]}
}

func (s *Store) SaveColor(c led.RGB) {
	_ = os.WriteFile(filepath.Join(s.Dir, "color.txt"),
		[]byte(fmt.Sprintf("%d %d %d", c.R, c.G, c.B)), 0o644)
}

// --- Админы ---

func (s *Store) loadAdmins() {
	b, err := os.ReadFile(filepath.Join(s.Dir, "admins.txt"))
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if id, err := strconv.ParseInt(line, 10, 64); err == nil {
			s.admins[id] = struct{}{}
		}
	}
}

func (s *Store) saveAdmins() {
	ids := s.ListAdmins()
	lines := make([]string, 0, len(ids))
	for _, id := range ids {
		lines = append(lines, strconv.FormatInt(id, 10))
	}
	_ = os.WriteFile(filepath.Join(s.Dir, "admins.txt"), []byte(strings.Join(lines, "\n")), 0o644)
}

func (s *Store) IsSuperAdmin(id int64) bool { return id == s.SuperAdmin }

func (s *Store) IsAdmin(id int64) bool {
	if id == s.SuperAdmin {
		return true
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.admins[id]
	return ok
}

func (s *Store) AddAdmin(id int64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.admins[id]; ok {
		return false
	}
	s.admins[id] = struct{}{}
	s.saveAdmins()
	return true
}

func (s *Store) RemoveAdmin(id int64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.admins[id]; !ok {
		return false
	}
	delete(s.admins, id)
	s.saveAdmins()
	return true
}

func (s *Store) ListAdmins() []int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]int64, 0, len(s.admins))
	for id := range s.admins {
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}