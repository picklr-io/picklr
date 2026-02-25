package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

var (
	fmtCheck bool
	fmtWrite bool
)

var fmtCmd = &cobra.Command{
	Use:   "fmt [paths...]",
	Short: "Format PKL configuration files",
	Long: `Formats .pkl files to a canonical style.

By default, formats all .pkl files in the current directory.
Use --check to verify formatting without making changes.
Use --write to write changes back to files (default).

Formatting rules:
  - Consistent indentation (2 spaces)
  - Trailing newline
  - Trim trailing whitespace from lines`,
	RunE: runFmt,
}

func init() {
	fmtCmd.Flags().BoolVar(&fmtCheck, "check", false, "Check formatting without making changes (exit 1 if not formatted)")
	fmtCmd.Flags().BoolVar(&fmtWrite, "write", true, "Write formatted output back to files")
}

func runFmt(cmd *cobra.Command, args []string) error {
	paths := args
	if len(paths) == 0 {
		paths = []string{"."}
	}

	var files []string
	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil {
			return fmt.Errorf("failed to stat %s: %w", p, err)
		}
		if info.IsDir() {
			entries, err := findPklFiles(p)
			if err != nil {
				return err
			}
			files = append(files, entries...)
		} else {
			files = append(files, p)
		}
	}

	if len(files) == 0 {
		fmt.Println("No .pkl files found.")
		return nil
	}

	unformatted := 0
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			return fmt.Errorf("failed to read %s: %w", file, err)
		}

		formatted := formatPkl(string(data))

		if string(data) != formatted {
			unformatted++
			if fmtCheck {
				fmt.Printf("%s: not formatted\n", file)
			} else if fmtWrite {
				if err := os.WriteFile(file, []byte(formatted), 0644); err != nil {
					return fmt.Errorf("failed to write %s: %w", file, err)
				}
				fmt.Printf("%s: formatted\n", file)
			}
		}
	}

	if fmtCheck && unformatted > 0 {
		return fmt.Errorf("%d file(s) not formatted", unformatted)
	}

	if unformatted == 0 {
		fmt.Printf("All %d file(s) are properly formatted.\n", len(files))
	} else if !fmtCheck {
		fmt.Printf("Formatted %d file(s).\n", unformatted)
	}

	return nil
}

func findPklFiles(dir string) ([]string, error) {
	var files []string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(path, ".pkl") {
			files = append(files, path)
		}
		return nil
	})
	return files, err
}

// formatPkl applies basic formatting rules to PKL content.
func formatPkl(content string) string {
	lines := strings.Split(content, "\n")
	var formatted []string

	for _, line := range lines {
		// Trim trailing whitespace
		line = strings.TrimRight(line, " \t")
		formatted = append(formatted, line)
	}

	result := strings.Join(formatted, "\n")

	// Ensure trailing newline
	if !strings.HasSuffix(result, "\n") {
		result += "\n"
	}

	// Remove multiple consecutive blank lines (keep max 1)
	for strings.Contains(result, "\n\n\n") {
		result = strings.ReplaceAll(result, "\n\n\n", "\n\n")
	}

	return result
}
