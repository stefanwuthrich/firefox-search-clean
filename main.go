package main

import (
	"bufio"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	_ "github.com/mattn/go-sqlite3"
)

// main is the entry point of the CLI tool.
func main() {
	// --- 1. Setup Flags ---
	defaultProfilePath, err := findDefaultFirefoxProfile()
	if err != nil {
		log.Printf("Warning: could not auto-detect default Firefox profile: %v. Please specify the path manually with --profile.", err)
	}

	profilePath := flag.String("profile", defaultProfilePath, "Path to the Firefox profile directory.")
	wordsFile := flag.String("words", "words.txt", "Path to the file containing words to delete (one per line).")
	dryRun := flag.Bool("dry-run", false, "Show what would be deleted without actually deleting it.")
	flag.Parse()

	if *profilePath == "" {
		log.Fatalf("Error: Firefox profile path is required. Please specify it using the --profile flag.")
	}

	dbPath := filepath.Join(*profilePath, "places.sqlite")

	// --- 2. Pre-flight Checks ---
	fmt.Println("🧹 Firefox History Cleaner")
	fmt.Println("---------------------------")
	fmt.Printf("Profile Path: %s\n", *profilePath)
	fmt.Printf("Database: %s\n", dbPath)
	fmt.Printf("Words File: %s\n", *wordsFile)
	fmt.Printf("Dry Run Mode: %t\n", *dryRun)
	fmt.Println("---------------------------")

	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		log.Fatalf("Error: Firefox database 'places.sqlite' not found at the specified path.")
	}

	// Check for a lock file. This is crucial.
	lockFilePath := filepath.Join(*profilePath, ".parentlock")
	if runtime.GOOS == "windows" {
		lockFilePath = filepath.Join(*profilePath, "parent.lock")
	}
	if _, err := os.Stat(lockFilePath); err == nil {
		log.Println("🔴 ERROR: Firefox appears to be running. Please close Firefox completely before running this tool to avoid database corruption.")
		// ask user if he wants to continue or exit
		fmt.Println("Do you want to continue? (y/n)")
		var response string
		fmt.Scan(&response)
		if response != "y" {
			os.Exit(1)
		}
	}

	// --- 3. Read Words ---
	words, err := readWordsFromFile(*wordsFile)
	if err != nil {
		log.Fatalf("Error reading words file: %v", err)
	}
	if len(words) == 0 {
		log.Fatalf("No words found in '%s'. Nothing to do.", *wordsFile)
	}
	fmt.Printf("Loaded %d words to search for.\n", len(words))

	// --- 4. Connect to DB ---
	db, err := sql.Open("sqlite3", fmt.Sprintf("file:%s?_journal_mode=WAL", dbPath))
	if err != nil {
		log.Fatalf("Error opening database: %v", err)
	}
	defer db.Close()

	// --- 5. Run Cleanup ---
	err = cleanupHistory(db, words, *dryRun)
	if err != nil {
		log.Fatalf("An error occurred during cleanup: %v", err)
	}

	if *dryRun {
		fmt.Println("\n✅ Dry run complete. No changes were made.")
	} else {
		fmt.Println("\n✅ History and autocomplete cleanup complete.")
	}
}

func readWordsFromFile(path string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var words []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" && !strings.HasPrefix(line, "#") {
			words = append(words, line)
		}
	}
	return words, scanner.Err()
}

// cleanupHistory finds and deletes history from moz_places, moz_historyvisits, and moz_inputhistory.
func cleanupHistory(db *sql.DB, words []string, dryRun bool) error {
	// --- Find Places to Delete ---
	var placesWhereClauses []string
	var placesArgs []interface{}
	for _, word := range words {
		placesWhereClauses = append(placesWhereClauses, "url LIKE ? OR title LIKE ?")
		likePattern := "%" + word + "%"
		placesArgs = append(placesArgs, likePattern, likePattern)
	}
	placesWhereSQL := strings.Join(placesWhereClauses, " OR ")

	findSQL := "SELECT id, url, title FROM moz_places WHERE " + placesWhereSQL
	fmt.Println("\n🔎 Searching for matching history entries...")
	rows, err := db.Query(findSQL, placesArgs...)
	if err != nil {
		return fmt.Errorf("error querying for places to delete: %w", err)
	}
	defer rows.Close()

	var placeIDs []int64
	var entriesToDelete [][2]string
	for rows.Next() {
		var id int64
		var url, title sql.NullString
		if err := rows.Scan(&id, &url, &title); err != nil {
			return fmt.Errorf("error scanning row: %w", err)
		}
		placeIDs = append(placeIDs, id)
		entriesToDelete = append(entriesToDelete, [2]string{url.String, title.String})
	}

	// Also check for autocomplete entries to report them in the dry run
	var inputWhereClauses []string
	var inputArgs []interface{}
	for _, word := range words {
		inputWhereClauses = append(inputWhereClauses, "input LIKE ?")
		inputArgs = append(inputArgs, "%"+word+"%")
	}
	inputWhereSQL := strings.Join(inputWhereClauses, " OR ")
	findInputSQL := "SELECT input FROM moz_inputhistory WHERE " + inputWhereSQL

	inputRows, err := db.Query(findInputSQL, inputArgs...)
	if err != nil {
		return fmt.Errorf("error querying for autocomplete entries: %w", err)
	}
	defer inputRows.Close()

	var autocompleteToDelete []string
	for inputRows.Next() {
		var input string
		if err := inputRows.Scan(&input); err != nil {
			return fmt.Errorf("error scanning input row: %w", err)
		}
		autocompleteToDelete = append(autocompleteToDelete, input)
	}

	if len(placeIDs) == 0 && len(autocompleteToDelete) == 0 {
		fmt.Println("No matching history or autocomplete entries found. Nothing to do.")
		return nil
	}

	if len(entriesToDelete) > 0 {
		fmt.Printf("Found %d unique URL(s) to delete:\n", len(entriesToDelete))
		for _, entry := range entriesToDelete {
			fmt.Printf("  - URL: %s (Title: %s)\n", entry[0], entry[1])
		}
	}
	if len(autocompleteToDelete) > 0 {
		fmt.Printf("Found %d autocomplete suggestion(s) to delete:\n", len(autocompleteToDelete))
		for _, entry := range autocompleteToDelete {
			fmt.Printf("  - Typed Input: %s\n", entry)
		}
	}

	if dryRun {
		return nil
	}

	fmt.Println("\n🗑️ Deleting entries...")
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("could not begin transaction: %w", err)
	}

	// --- Perform Deletions ---
	if len(placeIDs) > 0 {
		idPlaceholders := "?" + strings.Repeat(",?", len(placeIDs)-1)
		idArgs := make([]interface{}, len(placeIDs))
		for i, id := range placeIDs {
			idArgs[i] = id
		}

		// STEP 1: Delete from moz_historyvisits
		deleteVisitsSQL := "DELETE FROM moz_historyvisits WHERE place_id IN (" + idPlaceholders + ")"
		result, err := tx.Exec(deleteVisitsSQL, idArgs...)
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to delete from moz_historyvisits: %w", err)
		}
		visitsDeleted, _ := result.RowsAffected()
		fmt.Printf("- Deleted %d individual visit records (from moz_historyvisits).\n", visitsDeleted)

		// STEP 2: Delete from moz_places
		deletePlacesSQL := "DELETE FROM moz_places WHERE id IN (" + idPlaceholders + ")"
		result, err = tx.Exec(deletePlacesSQL, idArgs...)
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to delete from moz_places: %w", err)
		}
		placesDeleted, _ := result.RowsAffected()
		fmt.Printf("- Deleted %d unique URL entries (from moz_places).\n", placesDeleted)
	}

	// STEP 3: Delete from moz_inputhistory
	// This is the new step to clear autocomplete suggestions.
	if len(autocompleteToDelete) > 0 {
		deleteInputSQL := "DELETE FROM moz_inputhistory WHERE " + inputWhereSQL
		result, err := tx.Exec(deleteInputSQL, inputArgs...)
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to delete from moz_inputhistory: %w", err)
		}
		inputsDeleted, _ := result.RowsAffected()
		fmt.Printf("- Deleted %d autocomplete entries (from moz_inputhistory).\n", inputsDeleted)
	}

	return tx.Commit()
}

func findDefaultFirefoxProfile() (string, error) {
	var basePath, iniPath string
	var err error

	switch runtime.GOOS {
	case "windows":
		appData, err := os.UserConfigDir()
		if err != nil {
			return "", err
		}
		basePath = filepath.Join(appData, "Mozilla", "Firefox")
	case "darwin":
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		basePath = filepath.Join(homeDir, "Library", "Application Support", "Firefox")
	case "linux":
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		basePath = filepath.Join(homeDir, ".mozilla", "firefox")
	default:
		return "", errors.New("unsupported operating system")
	}

	iniPath = filepath.Join(basePath, "profiles.ini")
	if _, err := os.Stat(iniPath); err != nil {
		return "", fmt.Errorf("profiles.ini not found at %s", iniPath)
	}

	file, err := os.Open(iniPath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	var profileDir string
	isRelative := true
	inProfileBlock := false

	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "[Install") {
			inProfileBlock = true
		}
		if inProfileBlock && strings.HasPrefix(line, "Default=") {
			profileDir = strings.SplitN(line, "=", 2)[1]
			return filepath.Join(basePath, profileDir), nil
		}
		if strings.HasPrefix(line, "[") && !strings.HasPrefix(line, "[Install") {
			inProfileBlock = false
		}
	}

	file.Seek(0, 0)
	scanner = bufio.NewScanner(file)

	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "[Profile0]") {
			inProfileBlock = true
		}
		if inProfileBlock && strings.HasPrefix(line, "Path=") {
			profileDir = strings.SplitN(line, "=", 2)[1]
		}
		if inProfileBlock && strings.HasPrefix(line, "IsRelative=") {
			isRelative = strings.TrimSpace(strings.SplitN(line, "=", 2)[1]) == "1"
		}
	}

	if profileDir != "" {
		if isRelative {
			return filepath.Join(basePath, profileDir), nil
		}
		return profileDir, nil
	}

	return "", errors.New("could not determine default profile from profiles.ini")
}
