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
	"net/http"
	"path/filepath"
	"slices"
	"strings"
	"unicode"

	"github.com/wuweidict/wudict/internal/facet"
	"github.com/wuweidict/wudict/internal/fsx"
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

func groupOrderCompare(names map[string]string, locale string) func(a, b groupOrder) int {
	col := collate.New(language.Make(locale), collate.Loose, collate.Numeric)
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
	if err := s.syncLinkedGroups(entries); err != nil {
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
		if info.Name != "" {
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

// A linked group's saved membership is its last known snapshot. If a rule
// disappears, keeping that snapshot lets the group become editable intact.
func (s *Server) syncLinkedGroups(entries []*entry) error {
	p := s.reg.prefs
	p.editMu.Lock()
	defer p.editMu.Unlock()
	f, _ := p.data()
	changed := false
	groups := slices.Clone(f.Groups)
	dicts := slices.Clone(f.Dicts)
	names := groupOrderNames(dicts, entries)
	compare := groupOrderCompare(names, f.Language)
	for i, g := range groups {
		if g.CustomOrder == nil {
			resolveGroupOrderMode(&g, compare)
			groups[i] = g
			changed = true
		}
		if g.Filter == nil {
			continue
		}
		if !s.filterExists(*g.Filter) {
			if g.SelectedFilter == nil {
				g.SelectedFilter = g.Filter
			}
			g.Filter = nil
			groups[i] = g
			changed = true
			continue
		}
		members := s.filterMembers(*g.Filter, entries, names)
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
	}
	if changed {
		return p.mutate(func(dst *prefsFile) { dst.Groups = groups; dst.Dicts = dicts })
	}
	return nil
}

func (s *Server) handleCreateGroup(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name    string       `json:"name"`
		Filter  *groupFilter `json:"filter"`
		Linked  bool         `json:"linked"`
		Members []string     `json:"members"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		http.Error(w, "Invalid group name", 400)
		return
	}
	name := strings.TrimSpace(req.Name)
	if req.Linked && req.Filter == nil || req.Filter != nil && !s.filterExists(*req.Filter) {
		http.Error(w, "Filter not found", 400)
		return
	}
	if name == "" || len([]rune(name)) > 100 || strings.ContainsFunc(name, unicode.IsControl) {
		http.Error(w, "Enter a group name (1–100 characters, without control characters).", 400)
		return
	}
	p := s.reg.prefs
	// The name is validated against the record in effect and then written:
	// editMu keeps the two from interleaving with another writer.
	p.editMu.Lock()
	defer p.editMu.Unlock()
	f, _ := p.data()
	if strings.EqualFold(name, "All Dictionaries") {
		http.Error(w, "All Dictionaries is a reserved group name.", 409)
		return
	}
	for _, g := range f.Groups {
		if strings.EqualFold(g.Name, name) {
			http.Error(w, "A group with this name already exists.", 409)
			return
		}
	}
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		http.Error(w, "Could not create group", 500)
		return
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
				http.Error(w, "Dictionary no longer available", 404)
				return
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
		http.Error(w, "Could not save group: "+err.Error(), 500)
		return
	}
	writeJSON(w, viewGroup(g, append([]string{}, members...), false))
}

func (s *Server) handleRenameGroup(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Group  string       `json:"group"`
		Name   string       `json:"name"`
		Filter *groupFilter `json:"filter"`
		Linked *bool        `json:"linked"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil || req.Group == "" || req.Group == allDictionariesGroup {
		http.Error(w, "Invalid group", 400)
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" || len([]rune(name)) > 100 || strings.ContainsFunc(name, unicode.IsControl) {
		http.Error(w, "Enter a group name (1–100 characters, without control characters).", 400)
		return
	}
	if req.Linked != nil && *req.Linked && (req.Filter == nil || !s.filterExists(*req.Filter)) {
		http.Error(w, "Filter not found", 400)
		return
	}
	p := s.reg.prefs
	p.editMu.Lock()
	defer p.editMu.Unlock()
	f, _ := p.data()
	index := slices.IndexFunc(f.Groups, func(g DictionaryGroup) bool { return g.ID == req.Group })
	if index < 0 {
		http.Error(w, "Group not found", 404)
		return
	}
	if strings.EqualFold(name, "All Dictionaries") {
		http.Error(w, "All Dictionaries is a reserved group name.", 409)
		return
	}
	for _, g := range f.Groups {
		if g.ID != req.Group && strings.EqualFold(g.Name, name) {
			http.Error(w, "A group with this name already exists.", 409)
			return
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
		http.Error(w, "Could not save group: "+err.Error(), 500)
		return
	}
	writeJSON(w, map[string]string{"name": name})
}

// Delete only the selected user group. Dictionary records and their other
// memberships stay in place, including records for disconnected libraries.
func (s *Server) handleDeleteGroup(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Group string `json:"group"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil || req.Group == "" || req.Group == allDictionariesGroup {
		http.Error(w, "Invalid group", 400)
		return
	}
	p := s.reg.prefs
	p.editMu.Lock()
	defer p.editMu.Unlock()
	f, _ := p.data()
	if !slices.ContainsFunc(f.Groups, func(g DictionaryGroup) bool { return g.ID == req.Group }) {
		http.Error(w, "Group not found", 404)
		return
	}
	if err := p.mutate(func(f *prefsFile) {
		f.Groups = slices.DeleteFunc(f.Groups, func(g DictionaryGroup) bool { return g.ID == req.Group })
		for i := range f.Dicts {
			f.Dicts[i].Groups = slices.DeleteFunc(f.Dicts[i].Groups, func(id string) bool { return id == req.Group })
		}
	}); err != nil {
		http.Error(w, "Could not delete group: "+err.Error(), 500)
		return
	}
	writeJSON(w, map[string]bool{"deleted": true})
}

// Add a filtered set in one state-file update. Existing members and their
// order are retained; new members follow the order sent by the editor.
func (s *Server) handleAddGroupMembers(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Group   string   `json:"group"`
		Members []string `json:"members"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil || req.Group == "" || req.Group == allDictionariesGroup || req.Members == nil {
		http.Error(w, "Invalid membership", 400)
		return
	}
	entries := make(map[string]*entry, len(req.Members))
	for _, id := range req.Members {
		if _, exists := entries[id]; exists {
			continue
		}
		e, err := s.reg.get(id)
		if err != nil {
			http.Error(w, "Dictionary no longer available", 404)
			return
		}
		entries[id] = e
	}
	p := s.reg.prefs
	p.editMu.Lock()
	defer p.editMu.Unlock()
	f, _ := p.data()
	gi := slices.IndexFunc(f.Groups, func(g DictionaryGroup) bool { return g.ID == req.Group })
	if gi < 0 {
		http.Error(w, "Group not found", 404)
		return
	}
	if f.Groups[gi].Filter != nil {
		http.Error(w, "Linked group cannot be edited", 409)
		return
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
		http.Error(w, "Could not save membership: "+err.Error(), 500)
		return
	}
	writeJSON(w, map[string]bool{"saved": true})
}

func (s *Server) handleLinkGroup(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Group  string       `json:"group"`
		Filter *groupFilter `json:"filter"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil || req.Group == allDictionariesGroup || req.Filter != nil && !s.filterExists(*req.Filter) {
		http.Error(w, "Filter not found", 400)
		return
	}
	p := s.reg.prefs
	p.editMu.Lock()
	defer p.editMu.Unlock()
	f, _ := p.data()
	i := slices.IndexFunc(f.Groups, func(g DictionaryGroup) bool { return g.ID == req.Group })
	if i < 0 {
		http.Error(w, "Group not found", 404)
		return
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
		http.Error(w, "Could not save group: "+err.Error(), 500)
		return
	}
	writeJSON(w, map[string]bool{"saved": true})
}

// Set one membership, avoiding lost updates from clients editing other rows.
func (s *Server) handleGroupMember(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Group  string `json:"group"`
		Dict   string `json:"dict"`
		Member *bool  `json:"member"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil || req.Member == nil {
		http.Error(w, "Invalid membership", 400)
		return
	}
	if req.Group == allDictionariesGroup {
		http.Error(w, "All Dictionaries always contains every dictionary.", 400)
		return
	}
	e, err := s.reg.get(req.Dict)
	if err != nil {
		http.Error(w, "Dictionary no longer available", 404)
		return
	}
	p := s.reg.prefs
	p.editMu.Lock()
	defer p.editMu.Unlock()
	f, _ := p.data()
	gi := slices.IndexFunc(f.Groups, func(g DictionaryGroup) bool { return g.ID == req.Group })
	if gi < 0 {
		http.Error(w, "Group not found", 404)
		return
	}
	if f.Groups[gi].Filter != nil {
		http.Error(w, "Linked group cannot be edited", 409)
		return
	}
	dicts := slices.Clone(f.Dicts)
	i := slices.IndexFunc(dicts, func(d DictPref) bool { return d.ID == e.ID || fsx.SamePath(d.Path, e.Path) })
	if i < 0 {
		dicts = append(dicts, DictPref{ID: e.ID, Path: e.Path})
		i = len(dicts) - 1
	}
	if slices.Contains(dicts[i].Groups, req.Group) == *req.Member {
		writeJSON(w, map[string]bool{"saved": true})
		return
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
		http.Error(w, "Could not save membership: "+err.Error(), 500)
		return
	}
	writeJSON(w, map[string]bool{"saved": true})
}

// ReplaceGroupMembership applies a group's staged membership and visible order
// in one preference write. Members that disappeared from the registry while
// the editor was open are retained, so a rescan cannot erase hidden entries.
func (s *Server) handleReplaceGroupMembership(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Group       string   `json:"group"`
		Members     []string `json:"members"`
		Pinned      []string `json:"pinned"`
		CustomOrder *bool    `json:"customOrder"`
		ManualOrder bool     `json:"manualOrder"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil || req.Group == allDictionariesGroup || req.Members == nil {
		http.Error(w, "Invalid group membership", 400)
		return
	}
	p := s.reg.prefs
	entries := s.reg.all()
	p.editMu.Lock()
	defer p.editMu.Unlock()
	f, _ := p.data()
	gi := slices.IndexFunc(f.Groups, func(g DictionaryGroup) bool { return g.ID == req.Group })
	if gi < 0 {
		http.Error(w, "Group not found", 404)
		return
	}
	if f.Groups[gi].Filter != nil {
		http.Error(w, "Linked group cannot be edited", 409)
		return
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
			http.Error(w, "Invalid group membership", 400)
			return
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
				http.Error(w, "Invalid group pins", 400)
				return
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
		http.Error(w, "Could not save group membership: "+err.Error(), 500)
		return
	}
	writeJSON(w, map[string]bool{"saved": true})
}

func availableID(entries map[string]*entry, id string) bool { _, ok := entries[id]; return ok }

// Reorder only the currently available members. Unavailable dictionaries keep
// their membership and follow the visible rows until they reappear.
func (s *Server) handleGroupOrder(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Group       string   `json:"group"`
		Members     []string `json:"members"`
		Pinned      []string `json:"pinned"`
		CustomOrder *bool    `json:"customOrder"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil || req.Members == nil || req.Group == allDictionariesGroup {
		http.Error(w, "Invalid group order", 400)
		return
	}
	p := s.reg.prefs
	p.heal(s.reg)
	entries := s.reg.all()
	p.editMu.Lock()
	defer p.editMu.Unlock()
	f, _ := p.data()
	gi := slices.IndexFunc(f.Groups, func(g DictionaryGroup) bool { return g.ID == req.Group })
	if gi < 0 {
		http.Error(w, "Group not found", 404)
		return
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
		http.Error(w, "Group membership changed; reload the list", 409)
		return
	}
	seen := make(map[string]bool, len(req.Members))
	for _, id := range req.Members {
		if !visible[id] || seen[id] {
			http.Error(w, "Invalid group order", 400)
			return
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
				http.Error(w, "Invalid group order", 400)
				return
			}
			pinned[id] = true
		}
	}
	g.Order = groupOrderEntries(order, pinned)
	if !*g.CustomOrder {
		g.Order = sortedGroupOrder(g.Order, compare)
	}
	groupList[gi] = g
	if err := p.mutate(func(f *prefsFile) {
		f.Groups = groupList
	}); err != nil {
		http.Error(w, "Could not save group order: "+err.Error(), 500)
		return
	}
	writeJSON(w, map[string]bool{"saved": true})
}
