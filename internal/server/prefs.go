package server

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/wuweidict/wudict/internal/facet"
	"github.com/wuweidict/wudict/internal/fsx"
	"github.com/wuweidict/wudict/internal/logx"
)

// StateFile holds the part of the UI that describes the COLLECTION rather than
// the browser looking at it: which dictionaries are searched, in what order,
// and how big their text is set (see UIPrefs). Everything that describes the
// browser instead - theme, wide mode, the last dictionary picked in the
// dropdown - stays in localStorage, where it belongs.
//
// The split is not cosmetic. localStorage is keyed by origin, so
// scheme://host:port is part of the identity: changing SERVER_PORT, reaching
// the same server as 127.0.0.1 instead of localhost, or opening it from a
// phone each hands the user a blank slate. Curating a hundred dictionaries and
// losing it to a port change is not a preference that survived, it is a
// preference that was never stored. The server, which owns the dictionaries,
// owns the answer to "which of them do I search".
//
// It lives beside the wudict.toml in effect rather than at a hardcoded path,
// so a portable install (D32) keeps its state on the same stick as its config,
// and --config points both at once. It is deliberately NOT inside the library
// folders: D20 defines those as individually transferable units, and "which
// dictionaries are enabled" is a fact about the set, not about any member.
const StateFile = "state.json"

// prefsVersion is written to the file so a future format change can be
// recognised rather than guessed at.
const prefsVersion = 2

// DictPref is one dictionary's remembered state. The path is what makes this
// survivable: ids are sha256(path)[:12] (see pathID), so moving a dictionary
// folder changes every id at once. Recording the path - and, through it, the
// file name - lets the state be re-attached instead of silently reset.
type DictPref struct {
	Groups []string `json:"groups,omitempty"` // user group IDs; independent of Off
	ID     string   `json:"id"`
	Path   string   `json:"path"`
	Name   string   `json:"name,omitempty"`   // display name, for a readable file
	Off    bool     `json:"off,omitempty"`    // excluded from "All dictionaries"
	Pinned *bool    `json:"pinned,omitempty"` // fixed at the top when All Dictionaries is sorted
}

// UIPrefs is the part of the reading experience that belongs to the PERSON
// rather than to the browser in front of them. The split at the top of this
// file sends theme and wide mode to localStorage because they are per-device
// view choices; text size is not one. A reader who needs 24px needs it on the
// phone that reaches this same server too, and localStorage — keyed by origin
// — would hand them 15px there, and again the first time SERVER_PORT changes.
type UIPrefs struct {
	FontSize int `json:"fontSize,omitempty"` // article text, px; 0 = the default

	// FontWeight is the article text's weight, in CSS units. 0 is the zero
	// value AND the default (400), which is the same bargain as FontSize: a
	// half-written state.json gets the default rather than a wrong value.
	// Server state and not localStorage, for the reason the size is not - a
	// reader who needs 500 needs it on the phone that reaches this server too.
	FontWeight int `json:"fontWeight,omitempty"`

	// HLOff turns OFF marking of full-text matches inside articles. It is
	// negating for the same reason DictPref.Off is: the default is on, and a
	// default that is the zero value is a default a hand-written or
	// half-written state.json cannot get wrong. An absent key is the feature
	// working, never the feature silently missing.
	//
	// It is remembered state for the UI, not an instruction to the server: the
	// search path never reads it. The SPA turns it into ?hl= on the request,
	// which is what keeps an API client that never saw this file from
	// inheriting a preference set in somebody's browser.
	HLOff bool `json:"hlOff,omitempty"`

	// Also fold [ex] zones when the Examples Hide layer is enabled.
	HideUnmarkedExamples bool `json:"hideUnmarkedExamples,omitempty"`

	// FastFirst expands whichever dictionary answers first, instead of the
	// first dictionary in the user's own order that has a result (D73, D142;
	// default flipped in D144).
	//
	// Spelled as the opt-OUT, the way HLOff is, because the default is now the
	// reader's own order: the list they arranged is the answer to "which of
	// these do I want", and a race winner is not that answer. So an absent key
	// is the default strategy, which is the rule both flags obey - a
	// half-written state.json must not be able to invent a setting the user
	// never made.
	//
	// Like HLOff it is remembered state for the UI and not an instruction to
	// the server: /api/search knows nothing about it, because which section a
	// reader unfolds is a property of the reader, not of the answer.
	FastFirst bool `json:"fastFirst,omitempty"`

	// OpenN is how many sections open by themselves under the reader's own
	// order: the first OpenN dictionaries in that order that have a result.
	// 0 (absent) and 1 are both the default, one; there is no upper limit,
	// because the reader may trade speed for it. OpenAll opens every one that
	// has a result. The page's menu sets these and FastFirst together, and
	// FastFirst, which opens a single section, wins if a hand edit sets both.
	// Remembered state for the UI, like FastFirst: the server never reads it.
	OpenN   int  `json:"openN,omitempty"`
	OpenAll bool `json:"openAll,omitempty"`

	// A `sortMine` field lived here: list the picker A→Z, or in the user's own
	// panel order. The switch that wrote it is gone (the picker lists the
	// reader's arrangement and nothing else), so the field is too - a stored
	// true would otherwise describe a choice the app can no longer show or
	// undo. Unknown keys are ignored on read and dropped on the next save, so
	// a state.json written by an older build needs no migration.

	// SpeakOff turns OFF the read-aloud button that appears over a text
	// selection in an article. Negated like HLOff: the feature is on unless
	// the person said otherwise. Which VOICE reads is not here - the voices
	// are whatever the device in front of them has installed, so that choice
	// stays in that browser's localStorage.
	SpeakOff bool `json:"speakOff,omitempty"`

	// GroupsOff lists the picker sections the person has hidden, by facet id:
	// "lang", "pair", or a groups.ini section lower-cased (internal/facet).
	// Negated for the same reason as the flags above: every section the
	// library supports is shown unless the person said otherwise, and a facet
	// added tomorrow appears without a migration. Ids are kept opaque here - the server never groups anything,
	// and an id no facet uses any more simply hides nothing.
	GroupsOff []string `json:"groupsOff,omitempty"`

	// Full opens an article's secondary zone (DSL [*], wu-sec: examples and
	// links to sub-entries) shown instead of hidden. Spelled positively, like
	// SortMine: the brief view is Lingvo's own default, so the zero value
	// keeps it. It is the reader's last choice on a section's switch, and
	// sets how the next section opens; the server never reads it.
	Full bool `json:"full,omitempty"`

	// Mode is the search mode the reader last picked: "exact", "contains" or
	// "fts". Empty is prefix, the default, so an absent key keeps it. It is the
	// reader's habit, and which modes answer at all is a fact about this
	// server's library, so it lives here and not in the browser. The page
	// writes it only when the reader picks a mode, never when a link or the
	// history sets one; the server never reads it.
	Mode string `json:"mode,omitempty"`
}

// Article text-size bounds. The ceiling is deliberately past what the layout
// was drawn for: a reader who needs 32px needs it more than the design needs
// to stay pretty, and nothing in the article surface loses text when it grows
// — it only gets taller.
const (
	FontSizeMin = 11
	FontSizeMax = 32
)

// normalize repairs what a hand-edited file may carry. Clamping rather than
// zeroing, because "40" is a legible statement of intent that deserves the
// nearest size we offer, not a silent snap back to the default.
func (u *UIPrefs) normalize() {
	if u == nil {
		return
	}
	if u.FontSize != 0 {
		u.FontSize = max(FontSizeMin, min(FontSizeMax, u.FontSize))
	}
	u.GroupsOff = facetIDs(u.GroupsOff)
	if u.OpenN <= 1 {
		u.OpenN = 0 // one, the default, is stored as absent
	}
	switch u.Mode {
	case "exact", "contains", "fts":
	default: // prefix, and what a hand edit may carry (the retired "fuzzy", D16)
		u.Mode = ""
	}
}

// facetIDs bounds a hand-edited or hostile groupsOff to what a facet id can
// be: short, non-blank, unique, sorted so an unchanged set writes an unchanged
// file. Always a fresh slice, so a stored record never shares its backing
// array with the request it came from.
func facetIDs(in []string) []string {
	const maxIDs, maxLen = 32, 64 // ids are section names the user writes (D162)
	var out []string
	for _, id := range in {
		id = strings.TrimSpace(id)
		if id == "" || len(id) > maxLen || slices.Contains(out, id) {
			continue
		}
		if out = append(out, id); len(out) == maxIDs {
			break
		}
	}
	slices.Sort(out)
	return out
}

// prefsFile stores collection state. Unknown legacy parser-selection fields
// are ignored by JSON decoding and disappear on the next save.
type prefsFile struct {
	IndexDefaults *indexOptions           `json:"indexDefaults,omitempty"`
	IndexMigrated map[string]bool         `json:"indexMigrated,omitempty"` // completed legacy receipt migrations, per source
	DSLKnown      map[string]bool         `json:"dslKnown,omitempty"`
	DSLPending    map[string]indexOptions `json:"dslPending,omitempty"`
	DSLRemoved    map[string]bool         `json:"dslRemoved,omitempty"`
	Language      string                  `json:"language,omitempty"` // explicit interface language, never the dictionary language

	Groups  []DictionaryGroup `json:"groups,omitempty"`
	Version int               `json:"version"`
	UI      *UIPrefs          `json:"ui,omitempty"`
	Dicts   []DictPref        `json:"dicts"` // array ORDER is the user's order
}

// Prefs is the state file (state.json), an ownedFile (ownedfile.go): read
// bounded and re-read when it changes on disk, so a hand edit made while the
// app runs is adopted - including by the next save, which re-reads first and
// so builds on it instead of writing it away. A Prefs with an empty path is
// in-memory only: it answers questions and forgets on exit, which is what
// tests and a home-less environment need.
type Prefs struct {
	file  ownedFile[prefsFile]
	write sync.Mutex // a save's read-modify-write, whole

	// editMu serializes a whole read-decide-write sequence that spans more
	// than one call: heal() reads the records, decides, and writes them back,
	// and the DSL writers and /api/prefs do the same. The write lock inside
	// update()/mutate() only makes each WRITE atomic; this is what keeps two
	// such sequences from interleaving and losing one of them.
	editMu sync.Mutex
}

// maxPrefsBytes bounds state.json: thousands of dictionaries' worth, and a
// pipe or a device in its place is refused (fsx).
const maxPrefsBytes = 4 << 20

// LoadPrefs reads the state file. It never fails: a missing file is the normal
// first run, and an unreadable or corrupt one must not stop the app from
// serving dictionaries - the worst case is that the user re-curates a list.
func LoadPrefs(path string) *Prefs {
	p := &Prefs{}
	p.file = ownedFile[prefsFile]{
		name: StateFile, path: func() string { return path }, max: maxPrefsBytes,
		recheck: time.Second, perm: 0o600, parse: parsePrefs,
		absent: func() prefsFile { return prefsFile{} },
	}
	if st := p.file.now(); st.unusable() {
		logx.Warn("ignoring %s: %s", path, st.why)
	}
	return p
}

// parsePrefs reads a state file. A malformed one is announced (once per
// content: it is parsed only when it changed) and treated as no state - the
// next save writes over it, so this warning is the user's chance to notice.
func parsePrefs(text string) (prefsFile, []facet.Problem) {
	var f prefsFile
	if err := json.Unmarshal([]byte(text), &f); err != nil {
		logx.Warn("ignoring malformed %s: %v", StateFile, err)
		return prefsFile{}, []facet.Problem{{Msg: "malformed: " + err.Error()}}
	}
	f.UI.normalize()
	return f, nil
}

// data is the record in effect and whether it is a real one: a usable,
// well-formed file, or one this process wrote.
func (p *Prefs) data() (prefsFile, bool) {
	st := p.file.now()
	return st.val, st.exists && !st.unusable() && len(st.problems) == 0
}

// UI returns a copy of the UI record, or nil when nothing has been set. A copy
// because the caller marshals it while other requests may be writing.
func (p *Prefs) UI() *UIPrefs {
	f, _ := p.data()
	if f.UI == nil {
		return nil
	}
	u := *f.UI
	return &u
}

// Off reports whether this dictionary is excluded from "All dictionaries".
// It matches on id first and path second, so a record written before a move
// still speaks for the dictionary it was written about.
func (p *Prefs) Off(id, path string) bool {
	f, _ := p.data()
	for _, d := range f.Dicts {
		if d.ID == id || (d.Path != "" && fsx.SamePath(d.Path, path)) {
			return d.Off
		}
	}
	return false
}

// Snapshot returns the records and whether a state file exists.
func (p *Prefs) Snapshot() (dicts []DictPref, exists bool) {
	f, ok := p.data()
	return append([]DictPref(nil), f.Dicts...), ok
}

// Replace stores a new list and writes it, atomically (fsx.WriteAtomic): a
// crash mid-save leaves the previous state intact rather than a truncated
// file that reads as "nothing was ever configured".
func (p *Prefs) Replace(dicts []DictPref) error { return p.update(dicts, nil) }

// update is Replace plus an optional UI patch: the JSON object of the UI
// fields to change. A nil dicts keeps the stored list, decided here under the
// write lock rather than by the caller re-sending a snapshot it read before,
// which a save from another page could land between. Absent or null means "not mentioned" and leaves the stored
// record untouched, and inside the object the rule is the same per field - a
// key that is not sent keeps its stored value, and only a key sent as false,
// 0 or [] clears one. Two pages on one server (two tabs, or the Android app and
// its lookup popup, each a WebView of its own) each hold a copy of these
// settings read when they loaded; a page that sends only what it changed
// cannot carry its stale copy of the others back over a newer choice.
//
// One write, not two, so the two settings can never disagree on disk. It
// starts from the file as it is NOW (fresh): a hand edit made a moment ago is
// kept wherever this update does not speak.
func (p *Prefs) update(dicts []DictPref, ui json.RawMessage) error {
	p.write.Lock()
	defer p.write.Unlock()
	f := p.file.fresh().val
	f.Version = prefsVersion
	if dicts != nil {
		f.Dicts = append([]DictPref(nil), dicts...)
	}
	if mentioned(ui) {
		var u UIPrefs
		if f.UI != nil {
			u = *f.UI
			// json decodes an array into the slice it finds, in place: without
			// the clone the patch would write into the record the file cache
			// still holds
			u.GroupsOff = slices.Clone(u.GroupsOff)
		}
		if err := json.Unmarshal(ui, &u); err != nil {
			return err
		}
		u.normalize()
		f.UI = &u
	}
	return p.saveRecord(f)
}

// saveRecord writes a whole record, atomically, and installs it as the value
// in effect: the next read sees it without going back to the disk, and a
// concurrent writer's file is not overwritten by a stale copy (the caller
// starts from fresh()).
func (p *Prefs) saveRecord(f prefsFile) error {
	f.Version = prefsVersion
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	_, err = p.file.save(string(data)+"\n", true)
	return err
}

// mutate applies fn to the record in effect and saves the result. The record
// is a COPY: a caller that changes its mind simply does not save, and a failed
// save leaves the file - and the next read - exactly as they were, so the
// rollback every DSL writer used to spell out by hand is now the only possible
// outcome. Callers that read, decide and then write back must hold editMu.
func (p *Prefs) mutate(fn func(*prefsFile)) error {
	p.write.Lock()
	defer p.write.Unlock()
	f := p.file.fresh().val.clone()
	fn(&f)
	return p.saveRecord(f)
}

// store writes a record the caller built from data() while holding editMu.
// The lock covers the whole read-decide-write sequence, so replacing the
// record wholesale cannot lose a change another writer made in between - the
// alternative, mutate(), re-reads the file and is for changes that need no
// decision.
func (p *Prefs) store(f prefsFile) error {
	p.write.Lock()
	defer p.write.Unlock()
	return p.saveRecord(f)
}

// heal re-attaches stored records to the dictionaries as they exist NOW and
// persists the repair when anything moved.
//
// The identity ladder is id → path → unique file name, each rung weaker than
// the last and each guarded against collisions: a record may claim an entry
// only if no stronger record claimed it first, and the file-name rung is used
// only when that name is unique on BOTH sides. That last guard is what keeps
// library entries (every one of them a "text.db") from swapping identities
// with each other.
//
// Records that match nothing are kept, not dropped. An unplugged drive is the
// common case, and forgetting a user's curation because a disk was asleep
// would be exactly the failure this whole file exists to prevent.
func (p *Prefs) heal(r *Registry) []DictPref {
	p.editMu.Lock()
	defer p.editMu.Unlock()
	stored, _ := p.Snapshot()
	entries := r.all()

	byID := make(map[string]*entry, len(entries))
	byPath := make(map[string]*entry, len(entries))
	base := map[string][]*entry{}
	for _, e := range entries {
		byID[e.ID] = e
		if abs, err := filepath.Abs(e.Path); err == nil {
			byPath[filepath.Clean(abs)] = e
		} else {
			byPath[filepath.Clean(e.Path)] = e
		}
		b := strings.ToLower(filepath.Base(e.Path))
		base[b] = append(base[b], e)
	}
	// The other half of the file-name rung's guard: the name has to be unique
	// among the STORED records too. One
	// prepared dictionary and a handful of dead "…/text.db" records - the
	// residue of library folders the user has since removed - is the shape
	// where counting only the registry side lets a dead record adopt the live
	// entry, inheriting its off switch and its place in the order. Records a
	// stronger rung already claimed are counted as well: a name two records
	// share is ambiguous evidence whichever of them answered to it first.
	storedBase := map[string]int{}
	for _, d := range stored {
		if d.Path != "" {
			storedBase[strings.ToLower(filepath.Base(d.Path))]++
		}
	}

	out := append([]DictPref(nil), stored...)
	claimed := map[string]bool{}
	// strongest rung first, so a weaker match can never steal a live id
	for i, d := range out {
		if e, ok := byID[d.ID]; ok {
			claimed[e.ID] = true
			out[i].Path = e.Path
		}
	}
	changed := false
	for i, d := range out {
		if claimed[d.ID] {
			continue
		}
		var e *entry
		if abs, err := filepath.Abs(d.Path); err == nil && d.Path != "" {
			e = byPath[filepath.Clean(abs)]
		}
		if e == nil && d.Path != "" {
			b := strings.ToLower(filepath.Base(d.Path))
			if m := base[b]; len(m) == 1 && storedBase[b] == 1 {
				e = m[0]
			}
		}
		if e == nil || claimed[e.ID] {
			continue
		}
		claimed[e.ID] = true
		out[i].ID, out[i].Path = e.ID, e.Path
		changed = true
	}
	if changed {
		// The group orders are keyed by dictionary id, so a re-attached
		// record has to carry its memberships' ids with it or a group would
		// quietly lose the dictionary it was holding.
		remap := make(map[string]string)
		for i, d := range stored {
			if d.ID != out[i].ID {
				remap[d.ID] = out[i].ID
			}
		}
		p.write.Lock()
		f := p.file.fresh().val.clone()
		f.Dicts = out
		for i := range f.Groups {
			g := f.Groups[i]
			g.Order = slices.Clone(g.Order)
			for j, item := range g.Order {
				if replacement, ok := remap[item.ID]; ok {
					g.Order[j].ID = replacement
				}
			}
			f.Groups[i] = g
		}
		err := p.saveRecord(f)
		p.write.Unlock()
		if err != nil {
			logx.Warn("could not save %s: %v", p.file.path(), err)
		}
	}
	return out
}

// merge folds the client's ordered list into the stored one. The client can
// only speak for the dictionaries it can see, so records it did not mention
// are RETAINED at the end rather than deleted: an unmounted drive must not
// cost the user the settings for everything on it.
func (p *Prefs) merge(r *Registry, want []DictPref) []DictPref {
	stored, _ := p.Snapshot()
	out := make([]DictPref, 0, len(want)+len(stored))
	seenID := map[string]bool{}
	seenPath := map[string]bool{}
	for _, d := range want {
		if d.ID == "" || seenID[d.ID] {
			continue
		}
		// the registry, not the client, is authoritative about where a
		// dictionary lives; the name is the client echoing our own label back
		if e, err := r.get(d.ID); err == nil {
			d.Path = e.Path
		}
		if d.Name == "" && d.Path != "" {
			d.Name = filepath.Base(d.Path)
		}
		// The legacy preferences endpoint cannot change group membership.
		d.Groups = nil
		for _, old := range stored {
			if old.ID == d.ID || fsx.SamePath(old.Path, d.Path) {
				d.Groups = append([]string(nil), old.Groups...)
				if d.Pinned == nil {
					d.Pinned = old.Pinned
				}
				break
			}
		}
		seenID[d.ID] = true
		if d.Path != "" {
			seenPath[cleanAbs(d.Path)] = true
		}
		out = append(out, d)
	}
	for _, d := range stored {
		if seenID[d.ID] || (d.Path != "" && seenPath[cleanAbs(d.Path)]) {
			continue
		}
		out = append(out, d)
	}
	return out
}

// mentioned reports whether a request carried a UI patch: an absent key and
// an explicit null both leave the stored record alone.
func mentioned(ui json.RawMessage) bool {
	return len(ui) > 0 && string(ui) != "null"
}

func cleanAbs(p string) string {
	if abs, err := filepath.Abs(p); err == nil {
		return filepath.Clean(abs)
	}
	return filepath.Clean(p)
}

// GET /api/prefs - the enabled set and the order, healed against the
// dictionaries that exist right now. "exists" is false on a first run, which
// is the client's cue to adopt whatever an older build left in localStorage
// (once) instead of starting the user over.
func (s *Server) handlePrefs(w http.ResponseWriter, r *http.Request) {
	dicts := s.reg.prefs.heal(s.reg)
	_, exists := s.reg.prefs.Snapshot()
	writeJSON(w, map[string]any{"exists": exists, "dicts": dicts, "ui": s.reg.prefs.UI()})
}

// PUT /api/prefs - replace the order and enabled set when the request sends
// them, and change the UI fields it names (update). An absent or null dicts
// keeps the stored list: a page that changed only a UI setting must not carry
// its stale copy of the order back over another page's newer one.
func (s *Server) handleSavePrefs(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Dicts []DictPref      `json:"dicts"`
		UI    json.RawMessage `json:"ui"` // raw: which keys are present is the patch
	}
	if !decodeJSON(w, r, &req, 1<<20) {
		return
	}
	// merge reads the stored records and update writes them back: the pair is
	// one read-modify-write, so it is serialized as one.
	s.reg.prefs.editMu.Lock()
	defer s.reg.prefs.editMu.Unlock()
	// A wrongly typed field is the caller's mistake (400), found before the
	// save rather than inside it, where it would read as the server's (500).
	if mentioned(req.UI) {
		if err := json.Unmarshal(req.UI, new(UIPrefs)); err != nil {
			httpErr(w, http.StatusBadRequest, "bad request: %v", err)
			return
		}
	}
	var merged []DictPref
	if req.Dicts != nil {
		merged = s.reg.prefs.merge(s.reg, req.Dicts)
	}
	if err := s.reg.prefs.update(merged, req.UI); err != nil {
		httpErr(w, http.StatusInternalServerError, "%s", "could not save: "+err.Error())
		return
	}
	dicts, _ := s.reg.prefs.Snapshot()
	if dicts == nil {
		dicts = []DictPref{} // [] on the wire, as before, never null
	}
	writeJSON(w, map[string]any{"exists": true, "dicts": dicts, "ui": s.reg.prefs.UI()})
}
