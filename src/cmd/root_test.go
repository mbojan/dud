package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mitchellh/go-homedir"
	"github.com/spf13/viper"
)

func TestResolveRcloneConfig(t *testing.T) {
	t.Run("no key and no project file returns empty", func(t *testing.T) {
		viper.Reset()
		t.Cleanup(viper.Reset)

		rootDir := t.TempDir()
		got, err := resolveRcloneConfig(rootDir)
		if err != nil {
			t.Fatal(err)
		}
		if got != "" {
			t.Fatalf("expected empty string, got %q", got)
		}
	})

	t.Run("no key with project file returns absolute path", func(t *testing.T) {
		viper.Reset()
		t.Cleanup(viper.Reset)

		rootDir := t.TempDir()
		if err := os.MkdirAll(filepath.Join(rootDir, ".dud"), 0o755); err != nil {
			t.Fatal(err)
		}
		projectConf := filepath.Join(rootDir, ".dud", "rclone.conf")
		if err := os.WriteFile(projectConf, []byte{}, 0o644); err != nil {
			t.Fatal(err)
		}

		got, err := resolveRcloneConfig(rootDir)
		if err != nil {
			t.Fatal(err)
		}
		want, err := filepath.Abs(projectConf)
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("expected %q, got %q", want, got)
		}
	})

	t.Run("explicit absolute key wins over project file", func(t *testing.T) {
		viper.Reset()
		t.Cleanup(viper.Reset)

		rootDir := t.TempDir()
		if err := os.MkdirAll(filepath.Join(rootDir, ".dud"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(
			filepath.Join(rootDir, ".dud", "rclone.conf"),
			[]byte{},
			0o644,
		); err != nil {
			t.Fatal(err)
		}

		explicit := filepath.Join(t.TempDir(), "explicit.conf")
		viper.Set("rclone_config", explicit)

		got, err := resolveRcloneConfig(rootDir)
		if err != nil {
			t.Fatal(err)
		}
		if got != explicit {
			t.Fatalf("expected %q, got %q", explicit, got)
		}
	})

	t.Run("tilde in key gets home-expanded", func(t *testing.T) {
		viper.Reset()
		t.Cleanup(viper.Reset)

		viper.Set("rclone_config", "~/rclone.conf")

		got, err := resolveRcloneConfig(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		home, err := homedir.Dir()
		if err != nil {
			t.Fatal(err)
		}
		want := filepath.Join(home, "rclone.conf")
		if got != want {
			t.Fatalf("expected %q, got %q", want, got)
		}
		if strings.Contains(got, "~") {
			t.Fatalf("path %q still contains ~", got)
		}
	})

	t.Run("explicit empty string overrides project file", func(t *testing.T) {
		viper.Reset()
		t.Cleanup(viper.Reset)

		rootDir := t.TempDir()
		if err := os.MkdirAll(filepath.Join(rootDir, ".dud"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(
			filepath.Join(rootDir, ".dud", "rclone.conf"),
			[]byte{},
			0o644,
		); err != nil {
			t.Fatal(err)
		}

		viper.Set("rclone_config", "")

		got, err := resolveRcloneConfig(rootDir)
		if err != nil {
			t.Fatal(err)
		}
		if got != "" {
			t.Fatalf("expected empty override, got %q", got)
		}
	})
}
