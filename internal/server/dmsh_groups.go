// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

// The reader's own dictionary groups: named sets they curate by hand, one
// dictionary at a time, stored in state.json. This is the fork's half of the
// membership picker selected by the fork's CLI.
//
// Upstream's half is groups.go beside this file: groups.ini, the RULES that
// derive a group from a dictionary's own name, path and language. It stays
// available through its own API but does not feed the fork's picker. Separate
// API paths (/api/user-groups here, /api/groups there) let upstream's file be
// taken verbatim on every sync.
//
// The `dmsh_` prefix on this file's name is the fork's convention for a
// parallel implementation: upstream's file keeps its own name and merges
// cleanly, this one is never touched by upstream.

package server

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/wuweidict/wudict/internal/facet"
	"github.com/wuweidict/wudict/internal/fsx"
	"github.com/wuweidict/wudict/internal/logx"
	"golang.org/x/text/collate"
	"golang.org/x/text/language"
)

const allDictionariesGroup = "all"

// UseUserGroups selects the fork's membership picker at server startup.
// Upstream's rules editor and API stay available for synchronization, but its
// rules are separate editor filters; diagnostics do not enter the fork picker.
func (s *Server) UseUserGroups() { s.userGroupsOnly = true }

func (s *Server) pickerGroups(in facet.Input) []facet.Group {
	if s.userGroupsOnly {
		return nil
	}
	in.Rules = s.groupsNow().val
	return facet.Derive(in)
}

func (s *Server) pickerGroupsProblems() int {
	if s.userGroupsOnly {
		return 0
	}
	return facet.CountProblems(s.groupsNow().problems)
}

// DictionaryGroup is collection metadata in state.json. Membership is stored
// on DictPref so the existing identity repair also repairs group membership.
type DictionaryGroup struct {
	ID             string       `json:"id"`
	Name           string       `json:"name"`
	Order          []groupOrder `json:"order,omitempty"` // member IDs and their pin state; independent of global order
	Filter         *groupFilter `json:"filter,omitempty"`
	SelectedFilter *groupFilter `json:"selectedFilter,omitempty"` // retained when the link is switched off
	// False sorts unpinned members alphabetically; true preserves manual order
	// and appends new members alphabetically. Nil is a legacy group whose mode
	// is inferred from its saved order before new members are merged.
	CustomOrder *bool `json:"customOrder,omitempty"`
}

type groupOrder struct {
	ID     string `json:"id"`
	Pinned bool   `json:"pinned,omitempty"`
}

// Older state files stored order as a string array. Accept those entries and
// write the combined ID/pin record from here on.
func (o *groupOrder) UnmarshalJSON(data []byte) error {
	if len(data) > 0 && data[0] == '"' {
		return json.Unmarshal(data, &o.ID)
	}
	type record groupOrder
	return json.Unmarshal(data, (*record)(o))
}

func groupOrderIDs(order []groupOrder) []string {
	ids := make([]string, len(order))
	for i, item := range order {
		ids[i] = item.ID
	}
	return ids
}

func groupOrderEntries(ids []string, pinned map[string]bool) []groupOrder {
	order := make([]groupOrder, len(ids))
	for i, id := range ids {
		order[i] = groupOrder{ID: id, Pinned: pinned[id]}
	}
	return order
}

func groupPinnedIDs(order []groupOrder) []string {
	var ids []string
	for _, item := range order {
		if item.Pinned {
			ids = append(ids, item.ID)
		}
	}
	return ids
}

func groupPinnedSet(order []groupOrder) map[string]bool {
	pinned := make(map[string]bool)
	for _, item := range order {
		if item.Pinned {
			pinned[item.ID] = true
		}
	}
	return pinned
}

func groupOrderNames(dicts []DictPref, entries []*entry) map[string]string {
	names := make(map[string]string, len(dicts)+len(entries))
	for _, d := range dicts {
		name := d.Name
		if name == "" && d.Path != "" {
			name = filepath.Base(d.Path)
		}
		if name != "" {
			names[d.ID] = name
		}
	}
	for _, e := range entries {
		if names[e.ID] == "" {
			names[e.ID] = filepath.Base(e.Path)
		}
	}
	return names
}

func groupOrderCompare(names map[string]string, _ string) func(a, b groupOrder) int {
	// Alphabetical dictionary order is a stable data order, not presentation
	// localization. Keep it identical to the English interface for every user.
	col := collate.New(language.English, collate.Loose, collate.Numeric)
	return func(a, b groupOrder) int {
		an, bn := names[a.ID], names[b.ID]
		if an == "" {
			an = a.ID
		}
		if bn == "" {
			bn = b.ID
		}
		if n := col.CompareString(an, bn); n != 0 {
			return n
		}
		return strings.Compare(a.ID, b.ID)
	}
}

// Pinned dictionaries keep their order at the head. Only the tail is sorted.
func sortedGroupOrder(order []groupOrder, compare func(a, b groupOrder) int) []groupOrder {
	pinned, tail := make([]groupOrder, 0, len(order)), make([]groupOrder, 0, len(order))
	for _, item := range order {
		if item.Pinned {
			pinned = append(pinned, item)
		} else {
			tail = append(tail, item)
		}
	}
	slices.SortStableFunc(tail, compare)
	return append(pinned, tail...)
}

func resolveGroupOrderMode(g *DictionaryGroup, compare func(a, b groupOrder) int) bool {
	if g.CustomOrder == nil {
		custom := !slices.Equal(g.Order, sortedGroupOrder(g.Order, compare))
		g.CustomOrder = &custom
	}
	return *g.CustomOrder
}

func addGroupOrderMembers(g *DictionaryGroup, ids []string, compare func(a, b groupOrder) int) {
	custom := resolveGroupOrderMode(g, compare)
	seen := make(map[string]bool, len(g.Order))
	for _, item := range g.Order {
		seen[item.ID] = true
	}
	var added []groupOrder
	for _, id := range ids {
		if !seen[id] {
			added = append(added, groupOrder{ID: id})
			seen[id] = true
		}
	}
	slices.SortStableFunc(added, compare)
	g.Order = append(g.Order, added...)
	if !custom {
		g.Order = sortedGroupOrder(g.Order, compare)
	}
}

type groupFilter struct {
	Facet string `json:"facet"`
	Value string `json:"value"`
}

// linkedGroupState is guarded by Prefs.editMu, which also serializes the
// read/decide/write sequence of linked-group synchronization.
type linkedGroupState struct {
	version                  uint64
	prefs                    string
	rules                    string
	prefsExists, rulesExists bool
	prefsWhy, rulesWhy       string
	roots                    string
	entries                  []linkedGroupEntry
}

type linkedGroupEntry struct {
	entry    *entry
	id, path string
	gen      uint64
}

func (state *linkedGroupState) current(prefs fileState[prefsFile], rules fileState[*facet.Rules], roots string, entries []*entry) bool {
	if state.version == 0 || state.prefs != prefs.text || state.rules != rules.text || state.prefsExists != prefs.exists || state.rulesExists != rules.exists || state.prefsWhy != prefs.why || state.rulesWhy != rules.why || state.roots != roots || len(state.entries) != len(entries) {
		return false
	}
	for i, e := range entries {
		if state.entries[i] != (linkedGroupEntry{e, e.ID, e.Path, e.gen.Load()}) {
			return false
		}
	}
	return true
}

func (state *linkedGroupState) update(prefs fileState[prefsFile], rules fileState[*facet.Rules], roots string, entries []linkedGroupEntry) {
	state.version++
	state.prefs, state.rules = prefs.text, rules.text
	state.prefsExists, state.rulesExists = prefs.exists, rules.exists
	state.prefsWhy, state.rulesWhy = prefs.why, rules.why
	state.roots = roots
	state.entries = entries
}

type groupView struct {
	DictionaryGroup
	Order    []string `json:"order,omitempty"`
	Pinned   []string `json:"pinned,omitempty"` // derived API view; persistence keeps the flag beside each ordered ID
	Members  []string `json:"members"`
	Readonly bool     `json:"readonly"`
}

func viewGroup(group DictionaryGroup, members []string, readonly bool) groupView {
	return groupView{DictionaryGroup: group, Order: groupOrderIDs(group.Order), Pinned: groupPinnedIDs(group.Order), Members: members, Readonly: readonly}
}

func (s *Server) handleUserGroups(w http.ResponseWriter, r *http.Request) {
	s.reg.prefs.heal(s.reg)
	entries := s.reg.all()
	if err := s.syncLinkedGroups(entries, false); err != nil {
		http.Error(w, "Could not update linked groups: "+err.Error(), 500)
		return
	}
	f, _ := s.reg.prefs.data()
	all := DictionaryGroup{ID: allDictionariesGroup, Name: "All Dictionaries"}
	if f.UI != nil {
		all.CustomOrder = f.UI.CustomOrder
	}
	allView := viewGroup(all, []string{}, true)
	views := []groupView{allView}
	for _, g := range f.Groups {
		views = append(views, viewGroup(g, []string{}, false))
	}
	for _, e := range entries {
		views[0].Members = append(views[0].Members, e.ID)
		for _, d := range f.Dicts {
			if d.ID != e.ID {
				continue
			}
			for i := 1; i < len(views); i++ {
				if slices.Contains(d.Groups, views[i].ID) {
					views[i].Members = append(views[i].Members, e.ID)
				}
			}
			break
		}
	}
	for i := 1; i < len(views); i++ {
		rank := make(map[string]int, len(views[i].Order))
		for n, id := range views[i].Order {
			rank[id] = n
		}
		slices.SortStableFunc(views[i].Members, func(a, b string) int {
			ai, aok := rank[a]
			bi, bok := rank[b]
			if aok && bok {
				return ai - bi
			}
			if aok {
				return -1
			}
			if bok {
				return 1
			}
			return 0
		})
	}
	writeJSON(w, views)
}

func (s *Server) userGroupsSnapshot(trace bool) ([]groupView, error) {
	started := time.Now()
	stage := func(name string, count int) {
		if trace {
			logx.SystemDiagnostic("dictionary_picker_go stage=%s elapsed_ms=%d count=%d", name, time.Since(started).Milliseconds(), count)
		}
	}
	stage("start", 0)
	s.reg.prefs.heal(s.reg)
	stage("prefs_heal", 0)
	entries := s.reg.all()
	stage("registry_read", len(entries))
	if err := s.syncLinkedGroups(entries, trace); err != nil {
		stage("linked_groups_error", len(entries))
		return nil, fmt.Errorf("Could not update linked groups: %w", err)
	}
	stage("linked_groups", len(entries))
	f, _ := s.reg.prefs.data()
	stage("prefs_read", len(f.Groups))
	all := DictionaryGroup{ID: allDictionariesGroup, Name: "All Dictionaries"}
	if f.UI != nil {
		all.CustomOrder = f.UI.CustomOrder
	}
	views := []groupView{viewGroup(all, []string{}, true)}
	for _, g := range f.Groups {
		views = append(views, viewGroup(g, []string{}, false))
	}
	for _, e := range entries {
		views[0].Members = append(views[0].Members, e.ID)
		for _, d := range f.Dicts {
			if d.ID == e.ID {
				for i := 1; i < len(views); i++ {
					if slices.Contains(d.Groups, views[i].ID) {
						views[i].Members = append(views[i].Members, e.ID)
					}
				}
				break
			}
		}
	}
	for i := 1; i < len(views); i++ {
		rank := make(map[string]int, len(views[i].Order))
		for n, id := range views[i].Order {
			rank[id] = n
		}
		slices.SortStableFunc(views[i].Members, func(a, b string) int {
			ai, aok := rank[a]
			bi, bok := rank[b]
			if aok && bok {
				return ai - bi
			}
			if aok {
				return -1
			}
			if bok {
				return 1
			}
			return 0
		})
	}
	stage("views_ready", len(views))
	return views, nil
}

func (s *Server) filterExists(ref groupFilter) bool {
	if ref.Facet == "lang" || ref.Facet == "pair" {
		return true
	}
	for _, f := range s.groupsNow().val.Facets() {
		if f.ID == ref.Facet {
			for _, g := range f.Groups {
				if g.ID == ref.Value {
					return true
				}
			}
		}
	}
	return false
}
func (s *Server) filterMembers(ref groupFilter, entries []*entry, names map[string]string) []string {
	var ids []string
	for _, e := range entries {
		info := s.baseDictInfo(e)
		if info.Name != "" && names != nil {
			names[e.ID] = info.Name
		}
		for _, f := range info.Filters {
			if f.F == ref.Facet && f.V == ref.Value {
				ids = append(ids, e.ID)
				break
			}
		}
	}
	return ids
}

func (s *Server) syncLinkedGroups(entries []*entry, trace bool) error {
	started := time.Now()
	var metadataTime, matchTime, updateTime time.Duration
	metadataReads, linkedGroups := 0, 0
	cached := false
	p := s.reg.prefs
	p.editMu.Lock()
	defer p.editMu.Unlock()
	defer func() {
		if trace {
			logx.SystemDiagnostic("dictionary_picker_groups version=%d cached=%t linked=%d dictionaries=%d metadata_reads=%d metadata_ms=%d match_ms=%d update_ms=%d total_ms=%d", s.linkedGroups.version, cached, linkedGroups, len(entries), metadataReads, metadataTime.Milliseconds(), matchTime.Milliseconds(), updateTime.Milliseconds(), time.Since(started).Milliseconds())
		}
	}()
	state := p.file.fresh()
	rules := s.groupsNow()
	roots := strings.Join(s.reg.Dirs(), "\x00")
	if s.linkedGroups.current(state, rules, roots, entries) {
		cached = true
		return nil
	}
	inputEntries := make([]linkedGroupEntry, len(entries))
	for i, e := range entries {
		inputEntries[i] = linkedGroupEntry{e, e.ID, e.Path, e.gen.Load()}
	}
	f, _ := p.data()
	changed := false
	groups := slices.Clone(f.Groups)
	dicts := slices.Clone(f.Dicts)
	names := groupOrderNames(dicts, entries)
	compare := groupOrderCompare(names, f.Language)
	infoCache := make(map[string]dictInfo, len(entries))
	for i, g := range groups {
		if g.CustomOrder == nil {
			resolveGroupOrderMode(&g, compare)
			groups[i] = g
			changed = true
		}
		if g.Filter == nil {
			continue
		}
		linkedGroups++
		if !s.filterExists(*g.Filter) {
			if g.SelectedFilter == nil {
				g.SelectedFilter = g.Filter
			}
			g.Filter = nil
			groups[i] = g
			changed = true
			continue
		}
		matchStarted := time.Now()
		metadataBefore := metadataTime
		var members []string
		for _, e := range entries {
			info, ok := infoCache[e.ID]
			if !ok {
				metadataStarted := time.Now()
				info = s.baseDictInfo(e)
				metadataTime += time.Since(metadataStarted)
				metadataReads++
				infoCache[e.ID] = info
				if info.Name != "" {
					names[e.ID] = info.Name
				}
			}
			for _, filter := range info.Filters {
				if filter.F == g.Filter.Facet && filter.V == g.Filter.Value {
					members = append(members, e.ID)
					break
				}
			}
		}
		matchTime += time.Since(matchStarted) - (metadataTime - metadataBefore)
		updateStarted := time.Now()
		want := make(map[string]bool, len(members))
		for _, id := range members {
			want[id] = true
		}
		for j := range dicts {
			if slices.Contains(dicts[j].Groups, g.ID) == want[dicts[j].ID] {
				continue
			}
			dicts[j].Groups = slices.DeleteFunc(slices.Clone(dicts[j].Groups), func(id string) bool { return id == g.ID })
			if want[dicts[j].ID] {
				dicts[j].Groups = append(dicts[j].Groups, g.ID)
			}
			changed = true
		}
		for _, e := range entries {
			if !want[e.ID] || slices.ContainsFunc(dicts, func(d DictPref) bool { return d.ID == e.ID }) {
				continue
			}
			dicts = append(dicts, DictPref{ID: e.ID, Path: e.Path, Groups: []string{g.ID}})
			changed = true
		}
		oldOrder := g.Order
		g.Order = slices.DeleteFunc(slices.Clone(g.Order), func(item groupOrder) bool { return !want[item.ID] })
		addGroupOrderMembers(&g, members, compare)
		if !slices.Equal(g.Order, oldOrder) {
			groups[i] = g
			changed = true
		}
		updateTime += time.Since(updateStarted)
	}
	if changed {
		if err := p.mutate(func(dst *prefsFile) { dst.Groups = groups; dst.Dicts = dicts }); err != nil {
			return err
		}
	}
	s.linkedGroups.update(p.file.now(), rules, roots, inputEntries)
	return nil
}

func (s *Server) handleCreateGroup(w http.ResponseWriter, r *http.Request) {
	var req createGroupRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		http.Error(w, "Invalid group name", 400)
		return
	}
	result, err := s.createGroup(req)
	if err != nil {
		writeGroupOperationError(w, err)
		return
	}
	writeJSON(w, result)
}

type createGroupRequest struct {
	Name    string       `json:"name"`
	Filter  *groupFilter `json:"filter"`
	Linked  bool         `json:"linked"`
	Members []string     `json:"members"`
}
type renameGroupRequest struct {
	Group  string       `json:"group"`
	Name   string       `json:"name"`
	Filter *groupFilter `json:"filter"`
	Linked *bool        `json:"linked"`
}
type deleteGroupRequest struct {
	Group string `json:"group"`
}
type groupOrderRequest struct {
	Group       string   `json:"group"`
	Members     []string `json:"members"`
	Pinned      []string `json:"pinned"`
	CustomOrder *bool    `json:"customOrder"`
	ManualOrder bool     `json:"manualOrder"`
}
type addGroupMembersRequest struct {
	Group   string   `json:"group"`
	Members []string `json:"members"`
}
type linkGroupRequest struct {
	Group  string       `json:"group"`
	Filter *groupFilter `json:"filter"`
}
type groupMemberRequest struct {
	Group  string `json:"group"`
	Dict   string `json:"dict"`
	Member *bool  `json:"member"`
}
type replaceGroupMembershipRequest struct {
	Group       string   `json:"group"`
	Members     []string `json:"members"`
	Pinned      []string `json:"pinned"`
	CustomOrder *bool    `json:"customOrder"`
	ManualOrder bool     `json:"manualOrder"`
}
type groupOperationError struct {
	status  int
	message string
}

func (e groupOperationError) Error() string { return e.message }
func groupOperationFailure(status int, message string) error {
	return groupOperationError{status: status, message: message}
}
func writeGroupOperationError(w http.ResponseWriter, err error) {
	var operation groupOperationError
	if errors.As(err, &operation) {
		http.Error(w, operation.message, operation.status)
		return
	}
	http.Error(w, err.Error(), 500)
}

func (s *Server) createGroup(req createGroupRequest) (groupView, error) {
	name := strings.TrimSpace(req.Name)
	if req.Linked && req.Filter == nil || req.Filter != nil && !s.filterExists(*req.Filter) {
		return groupView{}, groupOperationFailure(http.StatusBadRequest, "Filter not found")
	}
	if name == "" || len([]rune(name)) > 100 || strings.ContainsFunc(name, unicode.IsControl) {
		return groupView{}, groupOperationFailure(http.StatusBadRequest, "Enter a group name (1–100 characters, without control characters).")
	}
	p := s.reg.prefs
	// The name is validated against the record in effect and then written:
	// editMu keeps the two from interleaving with another writer.
	p.editMu.Lock()
	defer p.editMu.Unlock()
	f, _ := p.data()
	if strings.EqualFold(name, "All Dictionaries") {
		return groupView{}, groupOperationFailure(http.StatusConflict, "All Dictionaries is a reserved group name.")
	}
	for _, g := range f.Groups {
		if strings.EqualFold(g.Name, name) {
			return groupView{}, groupOperationFailure(http.StatusConflict, "A group with this name already exists.")
		}
	}
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return groupView{}, groupOperationFailure(http.StatusInternalServerError, "Could not create group")
	}
	alphabetical := false
	g := DictionaryGroup{ID: hex.EncodeToString(id[:]), Name: name, SelectedFilter: req.Filter, CustomOrder: &alphabetical}
	entries := s.reg.all()
	names := groupOrderNames(f.Dicts, entries)
	var members []string
	if req.Filter != nil {
		members = s.filterMembers(*req.Filter, entries, names)
		g.Order = groupOrderEntries(members, nil)
		if req.Linked {
			g.Filter = req.Filter
		}
	}
	if !req.Linked && req.Members != nil {
		members = make([]string, 0, len(req.Members))
		seen := make(map[string]bool, len(req.Members))
		for _, id := range req.Members {
			if seen[id] {
				continue
			}
			seen[id] = true
			e, err := s.reg.get(id)
			if err != nil {
				return groupView{}, groupOperationFailure(http.StatusNotFound, "Dictionary no longer available")
			}
			if title := s.baseDictInfo(e).Name; title != "" {
				names[id] = title
			}
			members = append(members, id)
		}
		g.Order = groupOrderEntries(members, nil)
	}
	g.Order = sortedGroupOrder(g.Order, groupOrderCompare(names, f.Language))
	members = groupOrderIDs(g.Order)
	if err := p.mutate(func(f *prefsFile) {
		f.Groups = append(slices.Clone(f.Groups), g)
		for _, id := range members {
			e, err := s.reg.get(id)
			if err != nil {
				continue
			}
			i := slices.IndexFunc(f.Dicts, func(d DictPref) bool { return d.ID == id })
			if i < 0 {
				f.Dicts = append(f.Dicts, DictPref{ID: id, Path: e.Path, Groups: []string{g.ID}})
			}
			if i >= 0 {
				f.Dicts[i].Groups = append(slices.Clone(f.Dicts[i].Groups), g.ID)
			}
		}
	}); err != nil {
		return groupView{}, groupOperationFailure(http.StatusInternalServerError, "Could not save group: "+err.Error())
	}
	return viewGroup(g, append([]string{}, members...), false), nil
}

func (s *Server) handleRenameGroup(w http.ResponseWriter, r *http.Request) {
	var req renameGroupRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil || req.Group == "" || req.Group == allDictionariesGroup {
		http.Error(w, "Invalid group", 400)
		return
	}
	result, err := s.renameGroup(req)
	if err != nil {
		writeGroupOperationError(w, err)
		return
	}
	writeJSON(w, result)
}

func (s *Server) renameGroup(req renameGroupRequest) (map[string]string, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" || len([]rune(name)) > 100 || strings.ContainsFunc(name, unicode.IsControl) {
		return nil, groupOperationFailure(http.StatusBadRequest, "Enter a group name (1–100 characters, without control characters).")
	}
	if req.Linked != nil && *req.Linked && (req.Filter == nil || !s.filterExists(*req.Filter)) {
		return nil, groupOperationFailure(http.StatusBadRequest, "Filter not found")
	}
	p := s.reg.prefs
	p.editMu.Lock()
	defer p.editMu.Unlock()
	f, _ := p.data()
	index := slices.IndexFunc(f.Groups, func(g DictionaryGroup) bool { return g.ID == req.Group })
	if index < 0 {
		return nil, groupOperationFailure(http.StatusNotFound, "Group not found")
	}
	if strings.EqualFold(name, "All Dictionaries") {
		return nil, groupOperationFailure(http.StatusConflict, "All Dictionaries is a reserved group name.")
	}
	for _, g := range f.Groups {
		if g.ID != req.Group && strings.EqualFold(g.Name, name) {
			return nil, groupOperationFailure(http.StatusConflict, "A group with this name already exists.")
		}
	}
	if err := p.mutate(func(f *prefsFile) {
		g := &f.Groups[index]
		g.Name = name
		if req.Linked != nil {
			if req.Filter != nil {
				g.SelectedFilter = req.Filter
			} else if g.SelectedFilter == nil {
				g.SelectedFilter = g.Filter
			}
			if *req.Linked {
				g.Filter = req.Filter
			} else {
				g.Filter = nil
			}
		}
	}); err != nil {
		return nil, groupOperationFailure(http.StatusInternalServerError, "Could not save group: "+err.Error())
	}
	return map[string]string{"name": name}, nil
}

// Delete only the selected user group. Dictionary records and their other
// memberships stay in place, including records for disconnected libraries.
func (s *Server) handleDeleteGroup(w http.ResponseWriter, r *http.Request) {
	var req deleteGroupRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil || req.Group == "" || req.Group == allDictionariesGroup {
		http.Error(w, "Invalid group", 400)
		return
	}
	if err := s.deleteGroup(req); err != nil {
		writeGroupOperationError(w, err)
		return
	}
	writeJSON(w, map[string]bool{"deleted": true})
}

func (s *Server) deleteGroup(req deleteGroupRequest) error {
	if req.Group == "" || req.Group == allDictionariesGroup {
		return groupOperationFailure(http.StatusBadRequest, "Invalid group")
	}
	p := s.reg.prefs
	p.editMu.Lock()
	defer p.editMu.Unlock()
	f, _ := p.data()
	if !slices.ContainsFunc(f.Groups, func(g DictionaryGroup) bool { return g.ID == req.Group }) {
		return groupOperationFailure(http.StatusNotFound, "Group not found")
	}
	if err := p.mutate(func(f *prefsFile) {
		f.Groups = slices.DeleteFunc(f.Groups, func(g DictionaryGroup) bool { return g.ID == req.Group })
		for i := range f.Dicts {
			f.Dicts[i].Groups = slices.DeleteFunc(f.Dicts[i].Groups, func(id string) bool { return id == req.Group })
		}
	}); err != nil {
		return groupOperationFailure(http.StatusInternalServerError, "Could not delete group: "+err.Error())
	}
	return nil
}

// Add a filtered set in one state-file update. Existing members and their
// order are retained; new members follow the order sent by the editor.
func (s *Server) handleAddGroupMembers(w http.ResponseWriter, r *http.Request) {
	var req addGroupMembersRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil || req.Group == "" || req.Group == allDictionariesGroup || req.Members == nil {
		http.Error(w, "Invalid membership", 400)
		return
	}
	if err := s.addGroupMembers(req); err != nil {
		writeGroupOperationError(w, err)
		return
	}
	writeJSON(w, map[string]bool{"saved": true})
}

func (s *Server) addGroupMembers(req addGroupMembersRequest) error {
	if req.Group == "" || req.Group == allDictionariesGroup || req.Members == nil {
		return groupOperationFailure(http.StatusBadRequest, "Invalid membership")
	}
	entries := make(map[string]*entry, len(req.Members))
	for _, id := range req.Members {
		if _, exists := entries[id]; exists {
			continue
		}
		e, err := s.reg.get(id)
		if err != nil {
			return groupOperationFailure(http.StatusNotFound, "Dictionary no longer available")
		}
		entries[id] = e
	}
	p := s.reg.prefs
	p.editMu.Lock()
	defer p.editMu.Unlock()
	f, _ := p.data()
	gi := slices.IndexFunc(f.Groups, func(g DictionaryGroup) bool { return g.ID == req.Group })
	if gi < 0 {
		return groupOperationFailure(http.StatusNotFound, "Group not found")
	}
	if f.Groups[gi].Filter != nil {
		return groupOperationFailure(http.StatusConflict, "Linked group cannot be edited")
	}
	names := groupOrderNames(f.Dicts, s.reg.all())
	for _, e := range entries {
		if title := s.baseDictInfo(e).Name; title != "" {
			names[e.ID] = title
		}
	}
	compare := groupOrderCompare(names, f.Language)
	if err := p.mutate(func(f *prefsFile) {
		g := &f.Groups[gi]
		var added []string
		for _, d := range f.Dicts {
			if slices.Contains(d.Groups, req.Group) && !slices.ContainsFunc(g.Order, func(item groupOrder) bool { return item.ID == d.ID }) {
				added = append(added, d.ID)
			}
		}
		for _, id := range req.Members {
			i := slices.IndexFunc(f.Dicts, func(d DictPref) bool { return d.ID == id || fsx.SamePath(d.Path, entries[id].Path) })
			if i < 0 {
				f.Dicts = append(f.Dicts, DictPref{ID: id, Path: entries[id].Path})
				i = len(f.Dicts) - 1
			}
			f.Dicts[i].ID, f.Dicts[i].Path = id, entries[id].Path
			if slices.Contains(f.Dicts[i].Groups, req.Group) {
				continue
			}
			f.Dicts[i].Groups = append(f.Dicts[i].Groups, req.Group)
			added = append(added, id)
		}
		addGroupOrderMembers(g, added, compare)
	}); err != nil {
		return groupOperationFailure(http.StatusInternalServerError, "Could not save membership: "+err.Error())
	}
	return nil
}

func (s *Server) handleLinkGroup(w http.ResponseWriter, r *http.Request) {
	var req linkGroupRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil || req.Group == allDictionariesGroup || req.Filter != nil && !s.filterExists(*req.Filter) {
		http.Error(w, "Filter not found", 400)
		return
	}
	if err := s.linkGroup(req); err != nil {
		writeGroupOperationError(w, err)
		return
	}
	writeJSON(w, map[string]bool{"saved": true})
}

func (s *Server) linkGroup(req linkGroupRequest) error {
	if req.Group == allDictionariesGroup || req.Filter != nil && !s.filterExists(*req.Filter) {
		return groupOperationFailure(http.StatusBadRequest, "Filter not found")
	}
	p := s.reg.prefs
	p.editMu.Lock()
	defer p.editMu.Unlock()
	f, _ := p.data()
	i := slices.IndexFunc(f.Groups, func(g DictionaryGroup) bool { return g.ID == req.Group })
	if i < 0 {
		return groupOperationFailure(http.StatusNotFound, "Group not found")
	}
	if err := p.mutate(func(dst *prefsFile) {
		g := &dst.Groups[i]
		if req.Filter != nil {
			g.SelectedFilter = req.Filter
		}
		if req.Filter == nil && g.SelectedFilter == nil {
			g.SelectedFilter = g.Filter
		}
		g.Filter = req.Filter
	}); err != nil {
		return groupOperationFailure(http.StatusInternalServerError, "Could not save group: "+err.Error())
	}
	return nil
}

// Set one membership, avoiding lost updates from clients editing other rows.
func (s *Server) handleGroupMember(w http.ResponseWriter, r *http.Request) {
	var req groupMemberRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil || req.Member == nil {
		http.Error(w, "Invalid membership", 400)
		return
	}
	if err := s.setGroupMember(req); err != nil {
		writeGroupOperationError(w, err)
		return
	}
	writeJSON(w, map[string]bool{"saved": true})
}

func (s *Server) setGroupMember(req groupMemberRequest) error {
	if req.Member == nil {
		return groupOperationFailure(http.StatusBadRequest, "Invalid membership")
	}
	if req.Group == allDictionariesGroup {
		return groupOperationFailure(http.StatusBadRequest, "All Dictionaries always contains every dictionary.")
	}
	e, err := s.reg.get(req.Dict)
	if err != nil {
		return groupOperationFailure(http.StatusNotFound, "Dictionary no longer available")
	}
	p := s.reg.prefs
	p.editMu.Lock()
	defer p.editMu.Unlock()
	f, _ := p.data()
	gi := slices.IndexFunc(f.Groups, func(g DictionaryGroup) bool { return g.ID == req.Group })
	if gi < 0 {
		return groupOperationFailure(http.StatusNotFound, "Group not found")
	}
	if f.Groups[gi].Filter != nil {
		return groupOperationFailure(http.StatusConflict, "Linked group cannot be edited")
	}
	dicts := slices.Clone(f.Dicts)
	i := slices.IndexFunc(dicts, func(d DictPref) bool { return d.ID == e.ID || fsx.SamePath(d.Path, e.Path) })
	if i < 0 {
		dicts = append(dicts, DictPref{ID: e.ID, Path: e.Path})
		i = len(dicts) - 1
	}
	if slices.Contains(dicts[i].Groups, req.Group) == *req.Member {
		return nil
	}
	groups := slices.Clone(dicts[i].Groups)
	groups = slices.DeleteFunc(groups, func(id string) bool { return id == req.Group })
	if *req.Member {
		groups = append(groups, req.Group)
	}
	dicts[i].Groups = groups
	groupList := slices.Clone(f.Groups)
	g := groupList[gi]
	names := groupOrderNames(f.Dicts, s.reg.all())
	if title := s.baseDictInfo(e).Name; title != "" {
		names[e.ID] = title
	}
	compare := groupOrderCompare(names, f.Language)
	custom := resolveGroupOrderMode(&g, compare)
	order := groupOrderIDs(g.Order)
	pinned := groupPinnedSet(g.Order)
	for _, d := range f.Dicts {
		if slices.Contains(d.Groups, req.Group) && !slices.Contains(order, d.ID) {
			order = append(order, d.ID)
		}
	}
	order = slices.DeleteFunc(order, func(id string) bool { return id == e.ID })
	delete(pinned, e.ID)
	if *req.Member {
		order = append(order, e.ID)
	}
	g.Order = groupOrderEntries(order, pinned)
	if !custom {
		g.Order = sortedGroupOrder(g.Order, compare)
	}
	groupList[gi] = g
	if err := p.mutate(func(f *prefsFile) {
		f.Dicts, f.Groups = dicts, groupList
	}); err != nil {
		return groupOperationFailure(http.StatusInternalServerError, "Could not save membership: "+err.Error())
	}
	return nil
}

// ReplaceGroupMembership applies a group's staged membership and visible order
// in one preference write. Members that disappeared from the registry while
// the editor was open are retained, so a rescan cannot erase hidden entries.
func (s *Server) handleReplaceGroupMembership(w http.ResponseWriter, r *http.Request) {
	var req replaceGroupMembershipRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil || req.Group == allDictionariesGroup || req.Members == nil {
		http.Error(w, "Invalid group membership", 400)
		return
	}
	if err := s.replaceGroupMembership(req); err != nil {
		writeGroupOperationError(w, err)
		return
	}
	writeJSON(w, map[string]bool{"saved": true})
}

func (s *Server) replaceGroupMembership(req replaceGroupMembershipRequest) error {
	if req.Group == allDictionariesGroup || req.Members == nil {
		return groupOperationFailure(http.StatusBadRequest, "Invalid group membership")
	}
	p := s.reg.prefs
	entries := s.reg.all()
	p.editMu.Lock()
	defer p.editMu.Unlock()
	f, _ := p.data()
	gi := slices.IndexFunc(f.Groups, func(g DictionaryGroup) bool { return g.ID == req.Group })
	if gi < 0 {
		return groupOperationFailure(http.StatusNotFound, "Group not found")
	}
	if f.Groups[gi].Filter != nil {
		return groupOperationFailure(http.StatusConflict, "Linked group cannot be edited")
	}
	available := make(map[string]*entry, len(entries))
	stored := make(map[string]bool)
	for _, dict := range f.Dicts {
		if slices.Contains(dict.Groups, req.Group) {
			stored[dict.ID] = true
		}
	}
	for _, entry := range entries {
		available[entry.ID] = entry
	}
	desired := make(map[string]bool, len(req.Members))
	for _, id := range req.Members {
		if _, ok := available[id]; !ok && !stored[id] || desired[id] {
			return groupOperationFailure(http.StatusBadRequest, "Invalid group membership")
		}
		desired[id] = true
	}
	dicts := slices.Clone(f.Dicts)
	for i := range dicts {
		if _, ok := available[dicts[i].ID]; !ok {
			continue
		}
		groups := slices.DeleteFunc(slices.Clone(dicts[i].Groups), func(id string) bool { return id == req.Group })
		if desired[dicts[i].ID] {
			groups = append(groups, req.Group)
		}
		dicts[i].Groups = groups
	}
	for _, entry := range entries {
		if desired[entry.ID] && !stored[entry.ID] {
			dicts = append(dicts, DictPref{ID: entry.ID, Path: entry.Path, Groups: []string{req.Group}})
		}
	}
	order := slices.Clone(req.Members)
	seen := make(map[string]bool, len(order))
	for _, id := range order {
		seen[id] = true
	}
	for _, item := range f.Groups[gi].Order {
		id := item.ID
		if stored[id] && !availableID(available, id) && !seen[id] {
			order = append(order, id)
			seen[id] = true
		}
	}
	for id := range stored {
		if !availableID(available, id) && !seen[id] {
			order = append(order, id)
		}
	}
	groupList := slices.Clone(f.Groups)
	g := groupList[gi]
	names := groupOrderNames(dicts, entries)
	for id := range desired {
		if !stored[id] {
			if entry := available[id]; entry != nil {
				if title := s.baseDictInfo(entry).Name; title != "" {
					names[id] = title
				}
			}
		}
	}
	compare := groupOrderCompare(names, f.Language)
	resolveGroupOrderMode(&g, compare)
	if req.CustomOrder != nil {
		g.CustomOrder = req.CustomOrder
	}
	pinned := groupPinnedSet(g.Order)
	for id := range pinned {
		if !desired[id] && (availableID(available, id) || !stored[id]) {
			delete(pinned, id)
		}
	}
	if req.Pinned != nil {
		retained := make(map[string]bool)
		for id := range pinned {
			if !availableID(available, id) {
				retained[id] = true
			}
		}
		pinned = retained
		for _, id := range req.Pinned {
			if !desired[id] || pinned[id] {
				return groupOperationFailure(http.StatusBadRequest, "Invalid group pins")
			}
			pinned[id] = true
		}
	}
	if *g.CustomOrder && !req.ManualOrder {
		old := make(map[string]bool, len(g.Order))
		for _, item := range g.Order {
			old[item.ID] = true
		}
		var existing, added []groupOrder
		for _, id := range order {
			item := groupOrder{ID: id, Pinned: pinned[id]}
			if old[id] {
				existing = append(existing, item)
			} else {
				added = append(added, item)
			}
		}
		slices.SortStableFunc(added, compare)
		g.Order = append(existing, added...)
	} else {
		g.Order = groupOrderEntries(order, pinned)
	}
	if !*g.CustomOrder {
		g.Order = sortedGroupOrder(g.Order, compare)
	}
	groupList[gi] = g
	if err := p.mutate(func(dst *prefsFile) { dst.Dicts, dst.Groups = dicts, groupList }); err != nil {
		return groupOperationFailure(http.StatusInternalServerError, "Could not save group membership: "+err.Error())
	}
	return nil
}

func availableID(entries map[string]*entry, id string) bool { _, ok := entries[id]; return ok }

// Reorder only the currently available members. Unavailable dictionaries keep
// their membership and follow the visible rows until they reappear.
func (s *Server) handleGroupOrder(w http.ResponseWriter, r *http.Request) {
	var req groupOrderRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil || req.Members == nil || req.Group == allDictionariesGroup {
		http.Error(w, "Invalid group order", 400)
		return
	}
	if err := s.saveGroupOrder(req); err != nil {
		writeGroupOperationError(w, err)
		return
	}
	writeJSON(w, map[string]bool{"saved": true})
}

func (s *Server) saveGroupOrder(req groupOrderRequest) error {
	if req.Members == nil || req.Group == allDictionariesGroup {
		return groupOperationFailure(http.StatusBadRequest, "Invalid group order")
	}
	p := s.reg.prefs
	p.heal(s.reg)
	entries := s.reg.all()
	p.editMu.Lock()
	defer p.editMu.Unlock()
	f, _ := p.data()
	gi := slices.IndexFunc(f.Groups, func(g DictionaryGroup) bool { return g.ID == req.Group })
	if gi < 0 {
		return groupOperationFailure(http.StatusNotFound, "Group not found")
	}
	visible := make(map[string]bool)
	for _, e := range entries {
		for _, d := range f.Dicts {
			if d.ID == e.ID && slices.Contains(d.Groups, req.Group) {
				visible[e.ID] = true
				break
			}
		}
	}
	if len(req.Members) != len(visible) {
		return groupOperationFailure(http.StatusConflict, "Group membership changed; reload the list")
	}
	seen := make(map[string]bool, len(req.Members))
	for _, id := range req.Members {
		if !visible[id] || seen[id] {
			return groupOperationFailure(http.StatusBadRequest, "Invalid group order")
		}
		seen[id] = true
	}
	groupList := slices.Clone(f.Groups)
	g := groupList[gi]
	names := groupOrderNames(f.Dicts, entries)
	for _, entry := range entries {
		if visible[entry.ID] {
			if title := s.baseDictInfo(entry).Name; title != "" {
				names[entry.ID] = title
			}
		}
	}
	compare := groupOrderCompare(names, f.Language)
	resolveGroupOrderMode(&g, compare)
	if req.CustomOrder != nil {
		g.CustomOrder = req.CustomOrder
	} else {
		current := make([]string, 0, len(visible))
		for _, item := range g.Order {
			if visible[item.ID] {
				current = append(current, item.ID)
			}
		}
		if len(current) == len(req.Members) && !slices.Equal(current, req.Members) {
			manual := true
			g.CustomOrder = &manual
		}
	}
	order := slices.Clone(req.Members)
	pinned := groupPinnedSet(g.Order)
	storedMembers := make(map[string]bool)
	for _, d := range f.Dicts {
		if slices.Contains(d.Groups, req.Group) {
			storedMembers[d.ID] = true
		}
	}
	for _, item := range g.Order {
		id := item.ID
		if storedMembers[id] && !seen[id] {
			order = append(order, id)
			seen[id] = true
		}
	}
	for _, d := range f.Dicts {
		if slices.Contains(d.Groups, req.Group) && !seen[d.ID] {
			order = append(order, d.ID)
			seen[d.ID] = true
		}
	}
	if req.Pinned != nil {
		pinned = make(map[string]bool, len(req.Pinned))
		for _, id := range req.Pinned {
			if !storedMembers[id] || pinned[id] {
				return groupOperationFailure(http.StatusBadRequest, "Invalid group order")
			}
			pinned[id] = true
		}
	}
	if *g.CustomOrder && !req.ManualOrder {
		old := make(map[string]bool, len(g.Order))
		for _, item := range g.Order {
			old[item.ID] = true
		}
		var existing, added []groupOrder
		for _, id := range order {
			item := groupOrder{ID: id, Pinned: pinned[id]}
			if old[id] {
				existing = append(existing, item)
			} else {
				added = append(added, item)
			}
		}
		slices.SortStableFunc(added, compare)
		g.Order = append(existing, added...)
	} else {
		g.Order = groupOrderEntries(order, pinned)
	}
	if !*g.CustomOrder {
		g.Order = sortedGroupOrder(g.Order, compare)
	}
	groupList[gi] = g
	if err := p.mutate(func(f *prefsFile) { f.Groups = groupList }); err != nil {
		return groupOperationFailure(http.StatusInternalServerError, "Could not save group order: "+err.Error())
	}
	return nil
}
