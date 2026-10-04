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
	"slices"
	"strings"
	"unicode"

	"github.com/wuweidict/wudict/internal/facet"
	"github.com/wuweidict/wudict/internal/fsx"
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
	ID    string   `json:"id"`
	Name  string   `json:"name"`
	Order []string `json:"order,omitempty"` // member IDs; independent of the global dictionary order
}

type groupView struct {
	DictionaryGroup
	Members  []string `json:"members"`
	Readonly bool     `json:"readonly"`
}

func (s *Server) handleUserGroups(w http.ResponseWriter, r *http.Request) {
	s.reg.prefs.heal(s.reg)
	entries := s.reg.all()
	f, _ := s.reg.prefs.data()
	all := groupView{DictionaryGroup: DictionaryGroup{ID: allDictionariesGroup, Name: "All Dictionaries"}, Members: []string{}, Readonly: true}
	views := []groupView{all}
	for _, g := range f.Groups {
		views = append(views, groupView{DictionaryGroup: g, Members: []string{}})
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

func (s *Server) handleCreateGroup(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil {
		http.Error(w, "Invalid group name", 400)
		return
	}
	name := strings.TrimSpace(req.Name)
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
	g := DictionaryGroup{ID: hex.EncodeToString(id[:]), Name: name}
	if err := p.mutate(func(f *prefsFile) {
		f.Groups = append(slices.Clone(f.Groups), g)
	}); err != nil {
		http.Error(w, "Could not save group: "+err.Error(), 500)
		return
	}
	writeJSON(w, groupView{DictionaryGroup: g, Members: []string{}})
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
	if !slices.ContainsFunc(f.Groups, func(g DictionaryGroup) bool { return g.ID == req.Group }) {
		http.Error(w, "Group not found", 404)
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
	gi := slices.IndexFunc(groupList, func(g DictionaryGroup) bool { return g.ID == req.Group })
	g := groupList[gi]
	order := slices.Clone(g.Order)
	for _, d := range f.Dicts {
		if slices.Contains(d.Groups, req.Group) && !slices.Contains(order, d.ID) {
			order = append(order, d.ID)
		}
	}
	g.Order = slices.DeleteFunc(order, func(id string) bool { return id == e.ID })
	if *req.Member {
		g.Order = append(g.Order, e.ID)
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

// Reorder only the currently available members. Unavailable dictionaries keep
// their membership and follow the visible rows until they reappear.
func (s *Server) handleGroupOrder(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Group   string   `json:"group"`
		Members []string `json:"members"`
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
	order := slices.Clone(req.Members)
	storedMembers := make(map[string]bool)
	for _, d := range f.Dicts {
		if slices.Contains(d.Groups, req.Group) {
			storedMembers[d.ID] = true
		}
	}
	for _, id := range g.Order {
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
	g.Order = order
	groupList[gi] = g
	if err := p.mutate(func(f *prefsFile) {
		f.Groups = groupList
	}); err != nil {
		http.Error(w, "Could not save group order: "+err.Error(), 500)
		return
	}
	writeJSON(w, map[string]bool{"saved": true})
}
