package main

import (
	"bufio"
	"crypto/md5"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

var version = "1.0.0"
var osExit = os.Exit

// ---------- Config ----------

type Config struct {
	DBName      string
	Conn        string
	DumpFile    string
	OutputDir   string
	Mode        string
	Clean       bool
	NoDbPath    bool
	BlacklistDb string
	WhitelistDb string
	ExcludeObj  string
	AclFiles    bool
	MoveRoles   bool
	Quiet       bool
	DryRun      bool
	Version     bool
	TestSQL     string
}

// ---------- Logging ----------

func log(cfg *Config, format string, args ...interface{}) {
	if !cfg.Quiet {
		fmt.Fprintf(os.Stderr, format+"\n", args...)
	}
}

// ---------- Dependency check ----------

func checkDep(name string) {
	if _, err := exec.LookPath(name); err != nil {
		fmt.Fprintf(os.Stderr, "Error: required command not found: %s\n", name)
		osExit(1)
	}
}

// ---------- Main ----------

func main() {
	cfg := parseFlags()

	if cfg.DryRun {
		fmt.Fprintln(os.Stderr, "Dry-run mode: no files will be written")
	}

	if cfg.DumpFile == "" && cfg.DBName == "" && cfg.TestSQL == "" {
		fmt.Fprintln(os.Stderr, "Error: either --file (or --file -) or --db is required")
		osExit(1)
	}

	// Internal: detect dbname from dump if not given
	dbname := cfg.DBName

	if cfg.Clean && !cfg.DryRun {
		if err := os.RemoveAll(cfg.OutputDir); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to clean %s: %v\n", cfg.OutputDir, err)
		}
		log(cfg, "Cleaned output directory: %s", cfg.OutputDir)
	}
	if !cfg.DryRun {
		if err := os.MkdirAll(cfg.OutputDir, 0755); err != nil {
			fmt.Fprintf(os.Stderr, "Error creating output directory: %v\n", err)
			os.Exit(1)
		}
	}

	// Create dump if needed (skip for stdin and test-sql mode)
	isStdin := cfg.DumpFile == "-"
	tmpDir := ""
	dumpFile := cfg.DumpFile
	if cfg.TestSQL != "" || isStdin {
		// no dump needed
	} else if dumpFile == "" {
		checkDep("pg_dump")
		var err error
		tmpDir, err = os.MkdirTemp("", "pg_atropos_*")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error creating temp dir: %v\n", err)
			osExit(1)
		}
		defer func() { _ = os.RemoveAll(tmpDir) }()
		dumpFile = filepath.Join(tmpDir, "dump.pgdump")

		log(cfg, "Creating dump of database: %s", cfg.DBName)
		args := []string{"-Fc", "-f", dumpFile}
		if cfg.Conn != "" {
			args = append(args, strings.Fields(cfg.Conn)...)
		} else {
			args = append(args, "-d", cfg.DBName)
		}
		cmd := exec.Command("pg_dump", args...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			fmt.Fprintf(os.Stderr, "pg_dump failed: %v\n%s\n", err, string(out))
			osExit(1)
		}
		log(cfg, "Dump created: %s", dumpFile)
	}

	// Detect dbname from TOC if not provided (skip for stdin and test-sql mode)
	if dbname == "" && cfg.TestSQL == "" && !isStdin {
		dbname = detectDbname(dumpFile)
	}

	log(cfg, "Database: %s", orUnknown(dbname))
	log(cfg, "Mode: %s", cfg.Mode)

	// Check blacklist/whitelist
	if !checkDbFilter(dbname, cfg.BlacklistDb, cfg.WhitelistDb) {
		log(cfg, "Database '%s' is excluded by filter, skipping", dbname)
		return
	}

	// Source of SQL: pg_restore pipe or test SQL file
	var reader io.Reader
	if cfg.TestSQL != "" {
		f, err := os.Open(cfg.TestSQL)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error opening test SQL file: %v\n", err)
			osExit(1)
		}
		defer func() { _ = f.Close() }()
		reader = f
	} else {
		checkDep("pg_restore")
		args := []string{"-f", "-"}
		if !isStdin {
			args = append(args, dumpFile)
		}
		cmd := exec.Command("pg_restore", args...)
		if isStdin {
			cmd.Stdin = os.Stdin
		}
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error creating pipe: %v\n", err)
			osExit(1)
		}
		cmd.Stderr = os.Stderr
		if err := cmd.Start(); err != nil {
			fmt.Fprintf(os.Stderr, "pg_restore failed: %v\n", err)
			osExit(1)
		}
		reader = stdout
		defer func() {
			if err := cmd.Wait(); err != nil {
				fmt.Fprintf(os.Stderr, "pg_restore failed: %v\n", err)
				osExit(1)
			}
		}()
	}

	total := parseAndWrite(reader, cfg, dbname)

	if cfg.DryRun {
		fmt.Fprintf(os.Stderr, "Dry-run: would extract %d objects to %s\n", total, cfg.OutputDir)
	} else {
		log(cfg, "Extracted %d objects to: %s", total, cfg.OutputDir)
		log(cfg, "Done.")
	}
}

func orUnknown(s string) string {
	if s == "" {
		return "unknown"
	}
	return s
}

// ---------- Flag parsing ----------

func parseFlags() *Config {
	cfg := &Config{
		OutputDir:   "./output",
		Mode:        "origin",
		BlacklistDb: "^(template|postgres)",
	}

	// Extract hidden --test-sql before flag parsing so it doesn't appear in --help
	args := os.Args[1:]
	for i := 0; i < len(args); i++ {
		if args[i] == "--test-sql" && i+1 < len(args) {
			cfg.TestSQL = args[i+1]
			args = append(args[:i], args[i+2:]...)
			break
		}
	}

	fs := flag.NewFlagSet("pg_atropos", flag.ContinueOnError)
	fs.StringVar(&cfg.DBName, "db", "", "Database name to dump")
	fs.StringVar(&cfg.Conn, "conn", "", "PostgreSQL connection string")
	fs.StringVar(&cfg.DumpFile, "file", "", "Custom-format dump file (\"-\" for stdin)")
	fs.StringVar(&cfg.OutputDir, "output", "./output", "Output directory")
	fs.StringVar(&cfg.Mode, "mode", "origin", "Output mode: origin|custom")
	fs.BoolVar(&cfg.Clean, "clean", false, "Clean output directory before processing")
	fs.BoolVar(&cfg.NoDbPath, "no-db-path", false, "Don't include database name in output path")
	fs.StringVar(&cfg.BlacklistDb, "blacklist-db", "^(template|postgres)", "Exclude databases matching pattern")
	fs.StringVar(&cfg.WhitelistDb, "whitelist-db", "", "Only include databases matching pattern")
	fs.StringVar(&cfg.ExcludeObj, "exclude-obj", "", "Exclude object types matching pattern")
	fs.BoolVar(&cfg.AclFiles, "acl-files", false, "Save ACLs to separate .acl.sql files")
	fs.BoolVar(&cfg.MoveRoles, "move-roles", false, "Move role files under database directory")
	fs.BoolVar(&cfg.Quiet, "quiet", false, "Suppress informational output")
	fs.BoolVar(&cfg.Version, "version", false, "Print version and exit")
	fs.BoolVar(&cfg.DryRun, "dry-run", false, "Print what would be extracted without writing")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "pg_atropos v%s - Split PostgreSQL custom-format dumps into organized files\n\n", version)
		fmt.Fprintf(os.Stderr, "Usage:\n  pg_atropos [options]\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		fs.PrintDefaults()
	}

	_ = fs.Parse(args)

	if cfg.Version {
		fmt.Fprintf(os.Stderr, "pg_atropos v%s\n", version)
		osExit(0)
	}

	if cfg.Mode != "origin" && cfg.Mode != "custom" {
		fmt.Fprintf(os.Stderr, "Error: mode must be 'origin' or 'custom'\n")
		osExit(1)
	}

	return cfg
}

// ---------- DB name detection ----------

var reDbname = regexp.MustCompile(`^;.*dbname:\s*(.*)`)

func detectDbname(dumpFile string) string {
	cmd := exec.Command("pg_restore", "--list", dumpFile)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(out), "\n") {
		if m := reDbname.FindStringSubmatch(line); m != nil {
			return strings.TrimSpace(m[1])
		}
	}
	return ""
}

// ---------- Database filtering ----------

func checkDbFilter(dbname, blacklist, whitelist string) bool {
	if dbname == "" {
		return true
	}
	if whitelist != "" {
		matched, _ := regexp.MatchString(whitelist, dbname)
		if !matched {
			return false
		}
	}
	if blacklist != "" {
		matched, _ := regexp.MatchString(blacklist, dbname)
		if matched {
			return false
		}
	}
	return true
}

// ---------- Header parsing ----------

var reHeader = regexp.MustCompile(`^-- Name: (.*?); Type: (.*?); Schema: (.*?); Owner: `)

func parseHeader(line string) (name, objType, schema string) {
	m := reHeader.FindStringSubmatch(line)
	if m == nil {
		return "", "", ""
	}
	name = strings.Trim(strings.Trim(m[1], `"`), " ")
	objType = strings.Trim(m[2], " ")
	schema = strings.Trim(strings.Trim(m[3], `"`), " ")
	return
}

// ---------- Object writer ----------

var dirsCreated = make(map[string]bool)

func mkdirCached(dir string) {
	if !dirsCreated[dir] {
		if err := os.MkdirAll(dir, 0755); err != nil {
			fmt.Fprintf(os.Stderr, "Error creating directory %s: %v\n", dir, err)
			osExit(1)
		}
		dirsCreated[dir] = true
	}
}

func writeObject(cfg *Config, dbname, curName, curType, curSchema, content string, total *int) {
	if curType == "" || content == "" {
		return
	}

	// Exclude filter
	if cfg.ExcludeObj != "" {
		if matched, _ := regexp.MatchString(cfg.ExcludeObj, curType); matched {
			return
		}
	}

	// TABLE DATA is skipped
	if curType == "TABLE DATA" {
		return
	}

	// Strip trailing newline from content accumulation
	content = strings.TrimSuffix(content, "\n")

	// Extract ACL/COMMENT object qualifier ("TABLE users" -> obj="users")
	objRest := curName
	if curType == "COMMENT" || curType == "ACL" {
		if parts := strings.SplitN(curName, " ", 2); len(parts) == 2 {
			objRest = parts[1]
		}
	}

	// Build db path component
	dbpart := ""
	if dbname != "" && !cfg.NoDbPath {
		dbpart = dbname + "/"
	}

	// SCHEMA special case
	if curType == "SCHEMA" {
		dir := filepath.Join(cfg.OutputDir, dbpart+curName)
		fpath := filepath.Join(dir, curName+".sql")
		if !cfg.DryRun {
			mkdirCached(dir)
			appendToFile(fpath, content)
		}
		*total++
		if cfg.DryRun {
			fmt.Fprintf(os.Stderr, "  %s\n", fpath)
		}
		return
	}

	// Determine type dir and object filename
	typeDir := curType
	objName := curName

	if cfg.Mode == "custom" {
		// Custom mode: lowercase dirs
		switch curType {
		case "SEQUENCE OWNED BY", "SEQUENCE SET":
			typeDir = "sequence"
		case "FUNCTION", "PROCEDURE":
			typeDir = "function"
			objName = funcFilename(curName)
		case "COMMENT", "ACL":
			typeDir = "acl"
			objName = objRest
		default:
			typeDir = strings.ToLower(curType)
		}
	} else {
		// Origin mode
		switch curType {
		case "FUNCTION", "PROCEDURE":
			objName = funcFilename(curName)
		case "COMMENT", "ACL":
			objName = objRest
		}
	}

	if objName == "" {
		return
	}

	// Schema dir: schema "-" objects with --move-roles skip the schema dir
	schemaDir := curSchema
	if cfg.MoveRoles && schemaDir == "-" {
		schemaDir = ""
	}

	// Build final path
	relPath := filepath.Join(dbpart, schemaDir, typeDir, objName+".sql")
	fullPath := filepath.Join(cfg.OutputDir, relPath)

	if cfg.AclFiles && curType == "ACL" {
		fullPath = strings.TrimSuffix(fullPath, ".sql") + ".acl.sql"
	}

	if !cfg.DryRun {
		mkdirCached(filepath.Dir(fullPath))
		appendToFile(fullPath, content)
	}
	*total++
	if cfg.DryRun {
		fmt.Fprintf(os.Stderr, "  %s\n", fullPath)
	}
}

func appendToFile(path, content string) {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0640)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening file %s: %v\n", path, err)
		osExit(1)
	}
	defer func() { _ = f.Close() }()
	info, _ := f.Stat()
	if info != nil && info.Size() > 0 {
		if _, err := f.WriteString("\n"); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing to %s: %v\n", path, err)
			osExit(1)
		}
	}
	if _, err := f.WriteString(content); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing to %s: %v\n", path, err)
		osExit(1)
	}
	if _, err := f.WriteString("\n"); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing to %s: %v\n", path, err)
		osExit(1)
	}
}

// ---------- Function name helpers ----------

func funcArgsHash(args string) string {
	if args == "" {
		return "000000"
	}
	h := md5.Sum([]byte(args))
	return hex.EncodeToString(h[:])[:6]
}

func funcFilename(fname string) string {
	// Extract args: functionName(arg1, arg2) -> arg1, arg2
	parenIdx := strings.Index(fname, "(")
	if parenIdx < 0 {
		return fname
	}
	baseName := fname[:parenIdx]
	argsPart := fname[parenIdx+1 : len(fname)-1] // strip parens
	hash := funcArgsHash(argsPart)
	return baseName + "-" + hash
}

// ---------- Parser ----------

func parseAndWrite(rc io.Reader, cfg *Config, dbname string) int {
	scanner := bufio.NewScanner(rc)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	var curName, curType, curSchema string
	var content strings.Builder
	total := 0
	inHeader := true
	skipObj := false

	for scanner.Scan() {
		line := scanner.Text()

		// Check for header
		if name, typ, schema := parseHeader(line); name != "" {
			// Write previous object
			if curType != "" && !skipObj {
				writeObject(cfg, dbname, curName, curType, curSchema, content.String(), &total)
			}
			curName = name
			curType = typ
			curSchema = schema
			content.Reset()
			inHeader = false
			skipObj = (curType == "TABLE DATA")
			continue
		}

		if inHeader {
			continue
		}

		// Check for end marker
		if strings.HasPrefix(line, "-- PostgreSQL database dump complete") {
			if curType != "" && !skipObj {
				writeObject(cfg, dbname, curName, curType, curSchema, content.String(), &total)
			}
			curType = ""
			skipObj = false
			continue
		}

		// Accumulate content
		if !skipObj {
			// Skip leading blank lines
			if content.Len() == 0 && strings.TrimSpace(line) == "" {
				continue
			}
			if content.Len() > 0 {
				content.WriteString("\n")
			}
			content.WriteString(line)
		}
	}

	// Flush last object
	if curType != "" && !skipObj {
		writeObject(cfg, dbname, curName, curType, curSchema, content.String(), &total)
	}

	return total
}
