// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package logx

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func isolatedSystemLog(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "db")
	systemMu.Lock()
	old := systemRoot
	systemMu.Unlock()
	SetSystemLogRoot(func() string { return root })
	t.Cleanup(func() { SetSystemLogRoot(old) })
	return root + ".logs"
}

func TestSystemLogConcurrentWritesRotationAndSnapshots(t *testing.T) {
	isolatedSystemLog(t)
	const workers, records = 12, 50
	var wg sync.WaitGroup
	start := make(chan struct{})
	for worker := 0; worker < workers; worker++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			<-start
			for seq := 0; seq < records; seq++ {
				System("writer=%d seq=%d payload=%s", worker, seq, strings.Repeat("x", 4000))
			}
		}(worker)
	}
	close(start)
	for i := 0; i < 40; i++ {
		b, err := SystemSnapshot()
		if err != nil {
			t.Fatal(err)
		}
		if len(b) > 0 && b[len(b)-1] != '\n' {
			t.Fatal("snapshot contains partial record")
		}
	}
	wg.Wait()
	b, err := SystemSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	seen := make(map[string]bool)
	for _, line := range strings.Split(strings.TrimSuffix(string(b), "\n"), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 4 || fields[3] != "payload="+strings.Repeat("x", 4000) {
			t.Fatal("interleaved or truncated record")
		}
		key := fields[1] + " " + fields[2]
		if seen[key] {
			t.Fatalf("duplicate %s", key)
		}
		seen[key] = true
	}
	if len(seen) != workers*records {
		t.Fatalf("lost records: got %d", len(seen))
	}
	systemMu.Lock()
	current := systemSessionPath
	systemMu.Unlock()
	for _, path := range []string{current, current + ".1"} {
		st, err := os.Stat(path)
		if err != nil || st.Size() > SystemLogLimit {
			t.Fatalf("rotation size: %v", err)
		}
	}
}

func TestSystemLogWarningsRedactionAndUnavailableStorage(t *testing.T) {
	root := isolatedSystemLog(t)
	Warn("download https://user:secret@example.com/file?k=access-secret#fragment failed\nsecond line")
	b, err := SystemSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"user:secret", "access-secret", "fragment"} {
		if strings.Contains(string(b), secret) {
			t.Fatalf("secret leaked: %s", secret)
		}
	}
	if strings.Count(string(b), "\n") != 1 || !strings.Contains(string(b), `failed\nsecond line`) {
		t.Fatal("multiline record was not escaped")
	}
	SetSystemLogRoot(func() string { return filepath.Join(root, "system.log", "blocked") })
	// An error recording an error must not panic or deadlock.
	System("%s", fmt.Sprint("original failure"))
}

func TestSystemLogKeepsFiveSessionsAcrossRestart(t *testing.T) {
	root := isolatedSystemLog(t)
	for generation := 0; generation < 7; generation++ {
		SetSystemLogRoot(func() string { return strings.TrimSuffix(root, ".logs") })
		if err := StartSystemLogSession(); err != nil {
			t.Fatal(err)
		}
		System("generation=%d", generation)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	var sessions []string
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "system_") && strings.HasSuffix(entry.Name(), ".log") && !strings.Contains(entry.Name(), ".log.") {
			sessions = append(sessions, entry.Name())
		}
	}
	if len(sessions) != SystemLogFiles {
		t.Fatalf("retained %d sessions, want %d: %v", len(sessions), SystemLogFiles, sessions)
	}
	snapshot, err := SystemSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	for generation := 0; generation < 7-SystemLogFiles; generation++ {
		if strings.Contains(string(snapshot), fmt.Sprintf("generation=%d", generation)) {
			t.Fatalf("expired generation %d remains in export", generation)
		}
	}
	for generation := 7 - SystemLogFiles; generation < 7; generation++ {
		if !strings.Contains(string(snapshot), fmt.Sprintf("generation=%d", generation)) {
			t.Fatalf("export lost retained generation %d", generation)
		}
	}
}

func TestSystemLogProcessWriter(t *testing.T) {
	root := os.Getenv("WUDICT_TEST_SYSTEM_LOG")
	if root == "" {
		return
	}
	SetSystemLogRoot(func() string { return root })
	for n := 0; n < 200; n++ {
		System("pid=%d seq=%d payload=%s", os.Getpid(), n, strings.Repeat("p", 4000))
	}
}

func TestSystemLogMultipleProcesses(t *testing.T) {
	root := isolatedSystemLog(t)
	var commands []*exec.Cmd
	for i := 0; i < 3; i++ {
		cmd := exec.Command(os.Args[0], "-test.run=^TestSystemLogProcessWriter$", "-test.count=1")
		cmd.Env = append(os.Environ(), "WUDICT_TEST_SYSTEM_LOG="+strings.TrimSuffix(root, ".logs"))
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		commands = append(commands, cmd)
	}
	for n := 0; n < 20; n++ {
		b, err := SystemSnapshot()
		if err != nil {
			t.Fatal(err)
		}
		if len(b) > 0 && b[len(b)-1] != '\n' {
			t.Fatal("partial process record")
		}
	}
	for _, cmd := range commands {
		if err := cmd.Wait(); err != nil {
			t.Fatal(err)
		}
	}
	b, err := SystemSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	seen := make(map[string]bool)
	for _, line := range strings.Split(strings.TrimSuffix(string(b), "\n"), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 4 || fields[3] != "payload="+strings.Repeat("p", 4000) {
			t.Fatal("interleaved process record")
		}
		key := fields[1] + " " + fields[2]
		if seen[key] {
			t.Fatal("duplicate process record")
		}
		seen[key] = true
	}
	if len(seen) != 600 {
		t.Fatalf("lost process records: %d", len(seen))
	}
}
