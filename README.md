# Firefox Search Clean

A command-line tool that cleans Firefox search and visit history by removing entries containing specific words or phrases from a configurable word list.

## Features

- 🧹 Removes unwanted entries from Firefox history and autocomplete
- 🔍 Case-insensitive keyword matching
- 🛡️ Safe operation with dry-run mode
- 🔒 Automatic Firefox lock detection to prevent database corruption
- 📁 Auto-detects default Firefox profile path
- 📝 Configurable word list via text file

## Installation

### Prerequisites

- Go 1.18 or later
- Firefox browser (closed during cleanup)

### Build from Source

```bash
git clone https://github.com/stefanwuthrich/firefox-search-clean.git
cd firefox-search-clean
go mod tidy
go build -o firefox-search-clean
```

## Usage

### Basic Usage

```bash
# Run with default settings (auto-detects Firefox profile)
./firefox-search-clean

# Dry run to see what would be deleted without making changes
./firefox-search-clean --dry-run
```

### Command Line Options

```bash
./firefox-search-clean [OPTIONS]
```

| Option | Description | Default |
|--------|-------------|----------|
| `--profile` | Path to Firefox profile directory | Auto-detected |
| `--words` | Path to words file (one word/phrase per line) | `words.txt` |
| `--dry-run` | Show what would be deleted without deleting | `false` |

### Examples

```bash
# Use custom Firefox profile path
./firefox-search-clean --profile "/path/to/firefox/profile"

# Use custom words file
./firefox-search-clean --words "my-blocked-words.txt"

# Dry run with custom profile and words file
./firefox-search-clean --profile "/path/to/profile" --words "custom-words.txt" --dry-run
```

## Configuration

### Words File Format

Create a `words.txt` file (or specify a custom file with `--words`) containing words or phrases to remove, one per line:

```text
# List of words to remove from Firefox history.
# One word or phrase per line.
# The search is case-insensitive by default in SQLite's LIKE.

lottery
casino
unwanted-keyword
bingo
porn
```

- Lines starting with `#` are treated as comments
- Empty lines are ignored
- Search is case-insensitive
- Partial matches are found (e.g., "casino" matches "online-casino-games")

### Firefox Profile Location

The tool auto-detects your default Firefox profile, but you can specify a custom path:

**Linux/macOS:**
```bash
~/.mozilla/firefox/[profile-name]
```

**Windows:**
```bash
%APPDATA%\Mozilla\Firefox\Profiles\[profile-name]
```

## Safety Features

### Firefox Lock Detection

The tool automatically checks if Firefox is running by looking for lock files:
- Linux/macOS: `.parentlock`
- Windows: `parent.lock`

If Firefox is detected as running, you'll be prompted to close it before proceeding.

### Dry Run Mode

Always test with `--dry-run` first to see what would be deleted:

```bash
./firefox-search-clean --dry-run
```

Example output:
```
🧹 Firefox History Cleaner
---------------------------
Profile Path: /home/user/.mozilla/firefox/abc123.default
Database: /home/user/.mozilla/firefox/abc123.default/places.sqlite
Words File: words.txt
Dry Run Mode: true
---------------------------
Loaded 5 words to search for.

[DRY RUN] Would delete 15 history entries
[DRY RUN] Would delete 8 autocomplete entries

✅ Dry run complete. No changes were made.
```

## What Gets Cleaned

The tool removes entries from these Firefox database tables:

1. **`moz_places`** - Main history entries (URLs and titles)
2. **`moz_historyvisits`** - Visit records linked to places
3. **`moz_inputhistory`** - Autocomplete suggestions

## Troubleshooting

### Common Issues

**"Firefox database 'places.sqlite' not found"**
- Verify the Firefox profile path with `--profile`
- Ensure Firefox has been run at least once

**"Firefox appears to be running"**
- Close all Firefox windows and processes
- Wait a few seconds and try again

**"No words found in 'words.txt'"**
- Check that `words.txt` exists and contains non-comment lines
- Verify the file path with `--words`

### Backup Recommendation

While the tool is designed to be safe, consider backing up your Firefox profile before running:

```bash
cp -r ~/.mozilla/firefox/[profile-name] ~/.mozilla/firefox/[profile-name].backup
```

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## License

This project is open source. Please check the LICENSE file for details.
