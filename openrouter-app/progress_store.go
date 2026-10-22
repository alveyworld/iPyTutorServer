package main

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// A student's progress is kept on disk under their study code, so they can come back week
// after week, on any device, without an account. The code is the only key: it is random
// enough not to be guessed, and the login key already limits the site to the class.

const maxPracticeRecordsPerWeek = 60

type studentProfile struct {
	StudyCode   string                `json:"studyCode"`
	Major       string                `json:"major"`
	CreatedAt   time.Time             `json:"createdAt"`
	LastSeenAt  time.Time             `json:"lastSeenAt"`
	CurrentWeek int                   `json:"currentWeek,omitempty"`
	Weeks       map[int]*weekProgress `json:"weeks"`
}

type weekProgress struct {
	FirstStudied time.Time `json:"firstStudied"`
	LastStudied  time.Time `json:"lastStudied"`
	// LessonsDone holds the indexes of lessons the student worked through.
	LessonsDone []int            `json:"lessonsDone,omitempty"`
	Practice    []practiceRecord `json:"practice,omitempty"`
}

type practiceRecord struct {
	ID        string    `json:"id"`
	Kind      string    `json:"kind"`
	Title     string    `json:"title"`
	StartedAt time.Time `json:"startedAt"`
	Attempts  int       `json:"attempts"`
	HintsUsed int       `json:"hintsUsed,omitempty"`
	// SolutionShown is set when the student asked to see the worked solution.
	SolutionShown bool `json:"solutionShown,omitempty"`
	// Level is the tutor's latest interview-style rating of the attempt; empty until rated.
	Level string `json:"level,omitempty"`
}

// week returns the student's progress for a week, creating it on first use.
func (p *studentProfile) week(n int) *weekProgress {
	if p.Weeks == nil {
		p.Weeks = map[int]*weekProgress{}
	}
	wp := p.Weeks[n]
	if wp == nil {
		now := time.Now()
		wp = &weekProgress{FirstStudied: now, LastStudied: now}
		p.Weeks[n] = wp
	}
	return wp
}

func (wp *weekProgress) lessonDone(i int) bool {
	for _, done := range wp.LessonsDone {
		if done == i {
			return true
		}
	}
	return false
}

func (wp *weekProgress) markLessonDone(i int) {
	if !wp.lessonDone(i) {
		wp.LessonsDone = append(wp.LessonsDone, i)
	}
}

func (wp *weekProgress) record(id string) *practiceRecord {
	for i := range wp.Practice {
		if wp.Practice[i].ID == id {
			return &wp.Practice[i]
		}
	}
	return nil
}

func (wp *weekProgress) addRecord(rec practiceRecord) {
	wp.Practice = append(wp.Practice, rec)
	if over := len(wp.Practice) - maxPracticeRecordsPerWeek; over > 0 {
		wp.Practice = append([]practiceRecord(nil), wp.Practice[over:]...)
	}
}

// recentTitles lists the latest problem titles of a kind, newest first, so new problems
// can avoid repeating them.
func (wp *weekProgress) recentTitles(kind string, n int) []string {
	var titles []string
	for i := len(wp.Practice) - 1; i >= 0 && len(titles) < n; i-- {
		if wp.Practice[i].Kind == kind && wp.Practice[i].Title != "" {
			titles = append(titles, wp.Practice[i].Title)
		}
	}
	return titles
}

var studyCodePattern = regexp.MustCompile(`^[a-z]+-[a-z]+-[0-9]{4}$`)

var studyCodeSeparators = regexp.MustCompile(`[\s_]+|-{2,}`)

// normalizeStudyCode accepts a study code as a student might type it ("Maple River 4821",
// "`maple-river-4821`.") and returns its canonical form.
func normalizeStudyCode(text string) (string, bool) {
	s := strings.ToLower(strings.TrimSpace(text))
	s = strings.Trim(s, "`'\".,!?*() ")
	s = studyCodeSeparators.ReplaceAllString(s, "-")
	if !studyCodePattern.MatchString(s) {
		return "", false
	}
	return s, true
}

var studyCodeAdjectives = []string{
	"amber", "brave", "bright", "calm", "clever", "coral", "crisp", "daring", "eager", "early",
	"fancy", "fresh", "gentle", "golden", "happy", "honest", "jolly", "keen", "kind", "lively",
	"lucky", "merry", "mighty", "misty", "noble", "polite", "proud", "quick", "quiet", "rapid",
	"rosy", "shiny", "silver", "smart", "snowy", "steady", "sunny", "swift", "tidy", "witty",
}

var studyCodeNouns = []string{
	"anchor", "badger", "beacon", "birch", "canyon", "cedar", "comet", "coyote", "delta", "falcon",
	"fern", "forest", "garden", "glacier", "harbor", "heron", "island", "lantern", "maple", "meadow",
	"meteor", "orchid", "otter", "panda", "pebble", "pine", "planet", "prairie", "raven", "river",
	"rocket", "sparrow", "summit", "thunder", "tiger", "tulip", "valley", "walrus", "willow", "zebra",
}

func randomStudyCode() (string, error) {
	pick := func(n int) (int, error) {
		v, err := rand.Int(rand.Reader, big.NewInt(int64(n)))
		if err != nil {
			return 0, err
		}
		return int(v.Int64()), nil
	}
	a, err := pick(len(studyCodeAdjectives))
	if err != nil {
		return "", err
	}
	b, err := pick(len(studyCodeNouns))
	if err != nil {
		return "", err
	}
	d, err := pick(9000)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s-%s-%04d", studyCodeAdjectives[a], studyCodeNouns[b], d+1000), nil
}

var errUnknownStudyCode = errors.New("unknown study code")

// progressStore keeps profiles in memory and writes each change to dir/<code>.json.
type progressStore struct {
	dir      string
	mu       sync.Mutex
	profiles map[string]*studentProfile
}

func newProgressStore(dir string) (*progressStore, error) {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("create progress directory: %w", err)
	}
	return &progressStore{dir: dir, profiles: map[string]*studentProfile{}}, nil
}

func (s *progressStore) path(code string) string {
	return filepath.Join(s.dir, code+".json")
}

// loadLocked returns the cached or on-disk profile. code must already be normalized.
func (s *progressStore) loadLocked(code string) (*studentProfile, error) {
	if !studyCodePattern.MatchString(code) {
		return nil, errUnknownStudyCode
	}
	if p, ok := s.profiles[code]; ok {
		return p, nil
	}
	data, err := os.ReadFile(s.path(code))
	if errors.Is(err, os.ErrNotExist) {
		return nil, errUnknownStudyCode
	}
	if err != nil {
		return nil, err
	}
	var p studentProfile
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("read profile %s: %w", code, err)
	}
	s.profiles[code] = &p
	return &p, nil
}

func (s *progressStore) saveLocked(p *studentProfile) error {
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(s.dir, p.StudyCode+".*.tmp")
	if err != nil {
		return err
	}
	_, writeErr := tmp.Write(data)
	closeErr := tmp.Close()
	if writeErr != nil || closeErr != nil {
		os.Remove(tmp.Name())
		return errors.Join(writeErr, closeErr)
	}
	if err := os.Chmod(tmp.Name(), 0o640); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	if err := os.Rename(tmp.Name(), s.path(p.StudyCode)); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return nil
}

// create makes a profile with a new, unused study code.
func (s *progressStore) create(major string) (studentProfile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for tries := 0; tries < 20; tries++ {
		code, err := randomStudyCode()
		if err != nil {
			return studentProfile{}, err
		}
		if _, err := s.loadLocked(code); !errors.Is(err, errUnknownStudyCode) {
			continue // taken, or unreadable: pick another
		}
		now := time.Now()
		p := &studentProfile{StudyCode: code, Major: major, CreatedAt: now, LastSeenAt: now, Weeks: map[int]*weekProgress{}}
		if err := s.saveLocked(p); err != nil {
			return studentProfile{}, err
		}
		s.profiles[code] = p
		return copyProfile(p), nil
	}
	return studentProfile{}, errors.New("could not find an unused study code")
}

// get returns a copy of the profile for code.
func (s *progressStore) get(code string) (studentProfile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, err := s.loadLocked(code)
	if err != nil {
		return studentProfile{}, err
	}
	return copyProfile(p), nil
}

// update applies fn to the profile and saves it. fn must not keep the pointer.
func (s *progressStore) update(code string, fn func(p *studentProfile)) (studentProfile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, err := s.loadLocked(code)
	if err != nil {
		return studentProfile{}, err
	}
	fn(p)
	p.LastSeenAt = time.Now()
	if err := s.saveLocked(p); err != nil {
		return studentProfile{}, err
	}
	return copyProfile(p), nil
}

func copyProfile(p *studentProfile) studentProfile {
	data, err := json.Marshal(p)
	if err != nil {
		panic(err)
	}
	var out studentProfile
	if err := json.Unmarshal(data, &out); err != nil {
		panic(err)
	}
	return out
}
