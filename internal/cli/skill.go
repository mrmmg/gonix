package cli

import (
	"flag"
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"strconv"

	"github.com/mrmmg/gonix/skills"
)

func init() {
	register(
		command{path: "skill print", summary: "Print the AI agent skill (SKILL.md)", run: skillPrint},
		command{path: "skill install", args: "[--project | --dir DIR]",
			summary: "Install the skill for Claude Code (~/.claude/skills/gonix)", run: skillInstall},
	)
}

func skillPrint(e *env, args []string) error {
	if err := noFlags("skill print", args); err != nil {
		return err
	}
	fmt.Fprint(e.out, skills.GoNix)
	return nil
}

func skillInstall(e *env, args []string) error {
	fs := flag.NewFlagSet("skill install", flag.ContinueOnError)
	project := fs.Bool("project", false, "install into ./.claude/skills of the current directory")
	dir := fs.String("dir", "", "install into DIR/gonix instead")
	if _, err := parseFlags(fs, args); err != nil {
		return err
	}
	owner := invokingUser()
	base := *dir
	switch {
	case base != "":
	case *project:
		base = filepath.Join(".claude", "skills")
	default:
		home, err := homeOf(owner)
		if err != nil {
			return err
		}
		base = filepath.Join(home, ".claude", "skills")
	}
	target := filepath.Join(base, "gonix", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(target, []byte(skills.GoNix), 0o644); err != nil {
		return err
	}
	// Under sudo, hand the files to the user who ran it.
	if owner != nil && os.Geteuid() == 0 {
		uid, _ := strconv.Atoi(owner.Uid)
		gid, _ := strconv.Atoi(owner.Gid)
		for _, p := range []string{filepath.Dir(target), target} {
			_ = os.Chown(p, uid, gid)
		}
	}
	return e.show(map[string]any{"ok": true, "path": target}, func(w io.Writer) {
		fmt.Fprintf(w, "Installed the gonix skill to %s\n", target)
	})
}

// invokingUser is the user behind sudo, or nil when not run through sudo.
func invokingUser() *user.User {
	if name := os.Getenv("SUDO_USER"); name != "" {
		if u, err := user.Lookup(name); err == nil {
			return u
		}
	}
	return nil
}

func homeOf(u *user.User) (string, error) {
	if u != nil {
		return u.HomeDir, nil
	}
	return os.UserHomeDir()
}
