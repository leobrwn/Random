package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

var qdbusCommands = []string{"qdbus6", "qdbus-qt6", "qdbus-qt5", "qdbus"}

const sqliteBackupScript = `import sqlite3
import sys
from pathlib import Path

source_path = Path(sys.argv[1]).resolve()
backup_path = Path(sys.argv[2]).resolve()
source_uri = f"file:{source_path}?mode=ro"

try:
    source = sqlite3.connect(source_uri, uri=True)
    backup = sqlite3.connect(backup_path)
    source.backup(backup)

    # Make the backup a single self-contained SQLite file.
    backup.execute("PRAGMA journal_mode=DELETE")

    backup.close()
    source.close()
except sqlite3.Error as error:
    print(f"Could not back up Klipper's database: {error}", file=sys.stderr)
    sys.exit(1)
`

const jsonExportScript = `import json
import sqlite3
import sys
from pathlib import Path

sqlite_file = Path(sys.argv[1]).resolve()
active_clipboard_file = Path(sys.argv[2])
database_uri = f"file:{sqlite_file}?mode=ro"

try:
    active_text = active_clipboard_file.read_text(encoding="utf-8")
except (OSError, UnicodeDecodeError):
    active_text = ""

items = []

if active_text:
    items.append(active_text)

try:
    with sqlite3.connect(database_uri, uri=True) as database:
        rows = database.execute(
            """
            SELECT text
            FROM main
            WHERE text IS NOT NULL
              AND text <> ''
            ORDER BY last_used_time DESC, added_time DESC
            """
        )

        stored_item_count = 0

        for (text,) in rows:
            stored_item_count += 1

            if text not in items:
                items.append(text)

        print(
            f"Found {stored_item_count} stored text entries in Klipper's database.",
            file=sys.stderr,
        )

except sqlite3.Error as error:
    print(f"Could not read Klipper's database: {error}", file=sys.stderr)
    sys.exit(1)

# Store every clipboard item as an array of lines. This keeps line breaks
# visible in the JSON instead of representing them as \n inside one string.
formatted_items = [item.split("\n") for item in items]

json.dump(formatted_items, sys.stdout, ensure_ascii=False, indent=2)
sys.stdout.write("\n")
`

// findSQLiteFile picks one *.sqlite file from dir, mirroring
// `find dir -maxdepth 1 -type f -name '*.sqlite' -print -quit`.
func findSQLiteFile(dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	for _, e := range entries {
		if e.Type().IsRegular() && strings.HasSuffix(e.Name(), ".sqlite") {
			return filepath.Join(dir, e.Name()), nil
		}
	}
	return "", nil
}

func removeStaleTempFiles(store string) {
	patterns := []string{
		".klipper-active.*",
		".klipper-database.*",
		".klipper-history.json.*",
	}
	for _, pattern := range patterns {
		matches, _ := filepath.Glob(filepath.Join(store, pattern))
		for _, m := range matches {
			os.Remove(m)
		}
	}
}

// saveKlipperHistory asks Klipper to finish saving its history before the
// SQLite backup. Both stdout and stderr are discarded — it's fine if this
// fails or no D-Bus tool is available.
func saveKlipperHistory() bool {
	for _, cmd := range qdbusCommands {
		if _, err := exec.LookPath(cmd); err != nil {
			continue
		}
		if exec.Command(cmd, "org.kde.klipper", "/klipper", "saveClipboardHistory").Run() == nil {
			return true
		}
	}
	return false
}

// writeCommandOutput runs name/args, writing its stdout to destPath.
// Stderr is left visible, matching the original script's redirections.
func writeCommandOutput(name string, args []string, destPath string) bool {
	f, err := os.Create(destPath)
	if err != nil {
		return false
	}
	defer f.Close()

	cmd := exec.Command(name, args...)
	cmd.Stdout = f
	cmd.Stderr = os.Stderr
	return cmd.Run() == nil
}

// getActiveClipboard tries each clipboard backend in turn. It's valid for
// none to be available — the file is just left empty in that case.
func getActiveClipboard(path string) bool {
	// Wayland, text data only.
	if _, err := exec.LookPath("wl-paste"); err == nil {
		if writeCommandOutput("wl-paste", []string{"--type", "text/plain", "--no-newline"}, path) {
			return true
		}
	}

	// X11, text data only.
	if _, err := exec.LookPath("xclip"); err == nil {
		if writeCommandOutput("xclip", []string{"-selection", "clipboard", "-o", "-target", "UTF8_STRING"}, path) {
			return true
		}
	}

	if _, err := exec.LookPath("xsel"); err == nil {
		if writeCommandOutput("xsel", []string{"--clipboard", "--output"}, path) {
			return true
		}
	}

	// KDE's clipboard service.
	for _, cmd := range qdbusCommands {
		if _, err := exec.LookPath(cmd); err != nil {
			continue
		}
		if writeCommandOutput(cmd, []string{"org.kde.klipper", "/klipper", "getClipboardContents"}, path) {
			return true
		}
	}

	return false
}

func klipper12() {
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Could not determine home directory:", err)
		os.Exit(1)
	}

	sqlStorage := filepath.Join(home, ".local/share/klipper")
	store := filepath.Join(home, "klipperstore")

	if err := os.MkdirAll(store, 0755); err != nil {
		fmt.Fprintln(os.Stderr, "Could not create", store, ":", err)
		os.Exit(1)
	}

	// Remove temporary files left by an interrupted earlier run.
	removeStaleTempFiles(store)

	// Select one SQLite database file only.
	sourceFile, err := findSQLiteFile(sqlStorage)
	if err != nil || sourceFile == "" {
		fmt.Fprintf(os.Stderr, "No SQLite database found in %s\n", sqlStorage)
		os.Exit(1)
	}

	if _, err := exec.LookPath("python3"); err != nil {
		fmt.Fprintln(os.Stderr, "This backup requires python3.")
		os.Exit(1)
	}

	// Ask Klipper to finish saving its history before making the SQLite
	// backup. The SQLite backup below still works if Klipper's D-Bus
	// service is unavailable.
	saveKlipperHistory()

	// Klipper can store recent changes in a SQLite WAL file. The SQLite
	// backup API copies the live database correctly instead of copying
	// only one file.
	sqliteFile := filepath.Join(store, filepath.Base(sourceFile))

	tmpSqlite, err := os.CreateTemp(store, ".klipper-database.*")
	if err != nil {
		fmt.Fprintln(os.Stderr, "Could not create temporary file:", err)
		os.Exit(1)
	}
	temporarySqlite := tmpSqlite.Name()
	tmpSqlite.Close()

	backupCmd := exec.Command("python3", "-", sourceFile, temporarySqlite)
	backupCmd.Stdin = strings.NewReader(sqliteBackupScript)
	backupCmd.Stdout = os.Stdout
	backupCmd.Stderr = os.Stderr

	if err := backupCmd.Run(); err != nil {
		os.Remove(temporarySqlite)
		os.Remove(temporarySqlite + "-wal")
		os.Remove(temporarySqlite + "-shm")
		os.Exit(1)
	}

	// A previous backup may have left stale WAL files behind. They must
	// not be kept with the newly copied database, or SQLite can read the
	// wrong data.
	os.Remove(sqliteFile + "-wal")
	os.Remove(sqliteFile + "-shm")
	os.Remove(temporarySqlite + "-wal")
	os.Remove(temporarySqlite + "-shm")

	if err := os.Rename(temporarySqlite, sqliteFile); err != nil {
		fmt.Fprintln(os.Stderr, "Could not finalize backup:", err)
		os.Exit(1)
	}

	// The active clipboard may not be in Klipper's database yet.
	tmpActive, err := os.CreateTemp(store, ".klipper-active.*")
	if err != nil {
		fmt.Fprintln(os.Stderr, "Could not create temporary file:", err)
		os.Exit(1)
	}
	activeClipboardFile := tmpActive.Name()
	tmpActive.Close()
	defer os.Remove(activeClipboardFile)

	// It is valid for no clipboard command to be available. The database
	// backup still works, and the active clipboard file remains empty.
	getActiveClipboard(activeClipboardFile)

	jsonFile := filepath.Join(store, "klipper-history.json")
	tmpJSON, err := os.CreateTemp(store, ".klipper-history.json.*")
	if err != nil {
		fmt.Fprintln(os.Stderr, "Could not create temporary file:", err)
		os.Exit(1)
	}
	temporaryJSON := tmpJSON.Name()
	defer os.Remove(temporaryJSON)

	// Export the active text and every stored text entry. Image-only
	// entries have an empty text value, so they're excluded without
	// filtering MIME labels.
	exportCmd := exec.Command("python3", "-", sqliteFile, activeClipboardFile)
	exportCmd.Stdin = strings.NewReader(jsonExportScript)
	exportCmd.Stdout = tmpJSON
	exportCmd.Stderr = os.Stderr

	runErr := exportCmd.Run()
	tmpJSON.Close()

	if runErr != nil {
		os.Exit(1)
	}

	// Replace the previous backup only after the new JSON is complete.
	if err := os.Rename(temporaryJSON, jsonFile); err != nil {
		fmt.Fprintln(os.Stderr, "Could not finalize history export:", err)
		os.Exit(1)
	}
}
