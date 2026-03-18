package doltserver

import "testing"

func TestDeriveDBName(t *testing.T) {
	tests := []struct {
		name     string
		townRoot string
		workDir  string
		want     string
	}{
		{
			name:     "town root exact match returns hq",
			townRoot: "/home/user/gt",
			workDir:  "/home/user/gt",
			want:     "hq",
		},
		{
			name:     "town root with trailing slash returns hq",
			townRoot: "/home/user/gt",
			workDir:  "/home/user/gt/",
			want:     "hq",
		},
		{
			name:     "both have trailing slash returns hq",
			townRoot: "/home/user/gt/",
			workDir:  "/home/user/gt/",
			want:     "hq",
		},
		{
			name:     "rig directory returns rig name",
			townRoot: "/home/user/gt",
			workDir:  "/home/user/gt/gastown",
			want:     "gastown",
		},
		{
			name:     "rig directory with trailing slash",
			townRoot: "/home/user/gt",
			workDir:  "/home/user/gt/gastown/",
			want:     "gastown",
		},
		{
			name:     "nested rig subdirectory returns rig name",
			townRoot: "/home/user/gt",
			workDir:  "/home/user/gt/gastown/polecats/furiosa",
			want:     "gastown",
		},
		{
			name:     "double slash in path normalizes correctly",
			townRoot: "/home/user/gt",
			workDir:  "/home/user/gt//gastown",
			want:     "gastown",
		},
		{
			name:     "town root basename guard prevents returning gt",
			townRoot: "/home/user/gt",
			workDir:  "/home/user/gt/../gt",
			want:     "hq",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := deriveDBName(tt.townRoot, tt.workDir)
			if got != tt.want {
				t.Errorf("deriveDBName(%q, %q) = %q, want %q", tt.townRoot, tt.workDir, got, tt.want)
			}
		})
	}
}
