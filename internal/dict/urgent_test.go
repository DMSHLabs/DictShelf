// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package dict

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUrgentMarks(t *testing.T) {
	defer SetUrgentDefault(SetUrgentDefault(false))
	dir := t.TempDir()
	src := filepath.Join(dir, "a.wudict.md")
	if Urgent(src) {
		t.Fatal("unmarked source is urgent")
	}
	r1 := MarkUrgent(src)
	r2 := MarkUrgent(filepath.Join(dir, ".", "x", "..", "a.wudict.md")) // the same file, spelled otherwise
	if !Urgent(src) {
		t.Error("marked source is not urgent")
	}
	r1()
	r1() // a second release of the same mark is harmless
	if !Urgent(src) {
		t.Error("the other mark was released with the first")
	}
	r2()
	if Urgent(src) {
		t.Error("source still urgent after every mark was released")
	}
	SetUrgentDefault(true)
	if !Urgent(src) {
		t.Error("the default does not make an unmarked source urgent")
	}
}

func TestUrgentFolder(t *testing.T) {
	defer SetUrgentDefault(SetUrgentDefault(false))
	const name = "zz-urgent-main.test"
	fileOpeners[name] = func(string) (Dictionary, error) { return nil, os.ErrNotExist }
	defer delete(fileOpeners, name)
	dir := t.TempDir()
	main := filepath.Join(dir, name)
	if err := os.WriteFile(main, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	release := MarkUrgent(dir)
	if !Urgent(dir) || !Urgent(main) {
		t.Errorf("folder mark: folder %v, main file %v", Urgent(dir), Urgent(main))
	}
	release()
	if Urgent(main) {
		t.Error("main file still urgent after the folder's mark was released")
	}
}

func TestSelfPrepares(t *testing.T) {
	RegisterSelfPrepared(".ZZSELF")
	defer delete(selfPrepared, ".zzself")
	if !SelfPrepares("/x/y.zzself") || !SelfPrepares("Y.ZZSelf") {
		t.Error("a registered suffix is not self-preparing")
	}
	if SelfPrepares("/x/y.zzother") {
		t.Error("an unregistered suffix is self-preparing")
	}
}
