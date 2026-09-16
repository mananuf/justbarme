package app

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// LoadDotEnvIfPresent populates the process environment from a .env file in
// the current working directory, purely for local development convenience.
// It never overrides a variable already set in the real environment, and is
// a no-op if .env does not exist. config.Load() itself is unchanged and
// still only ever reads os.Getenv (docs/API_CONTRACT.md §5: config is
// environment-only) -- this just saves a developer from having to run
// `set -a && source .env && set +a` by hand before every `go run` command,
// which was the actual cause of "JBM_DATABASE_URL is required" when running
// a cmd/ binary directly in a fresh shell. Production and CI environments
// inject JBM_* variables directly and never carry a .env file, so this is
// skipped entirely there.
func LoadDotEnvIfPresent() error {
	f, err := os.Open(".env")
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("open .env: %w", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		if _, exists := os.LookupEnv(key); exists {
			continue
		}
		if err := os.Setenv(key, unquote(strings.TrimSpace(value))); err != nil {
			return fmt.Errorf("set %s from .env: %w", key, err)
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read .env: %w", err)
	}
	return nil
}

// unquote strips one matching pair of leading/trailing single or double
// quotes, the same way a shell's `source` (the documented manual workflow
// this replaces) would when evaluating VAR="value" -- without this, a
// quoted value passes through with the literal quote characters still
// attached, silently corrupting it for anything downstream that parses it
// as a URL, duration, etc.
func unquote(value string) string {
	if len(value) < 2 {
		return value
	}
	first, last := value[0], value[len(value)-1]
	if (first == '"' && last == '"') || (first == '\'' && last == '\'') {
		return value[1 : len(value)-1]
	}
	return value
}
